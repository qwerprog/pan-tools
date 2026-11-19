package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio/speeds"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
)

type (
	// Worker 下载工作单元
	Worker struct {
		totalSize    int64 // 整个文件的大小, 请求 range 时校验一致性
		wrange       *transfer.Range
		speedsStat   *speeds.Speeds
		id           int
		url          string
		referer      string
		acceptRanges string
		client       *requester.HTTPClient
		firstResp    *http.Response
		writerAt     io.WriterAt
		writeMu      *sync.Mutex
		execMu       sync.Mutex

		pauseChan              chan struct{}
		ctrlMu                 sync.Mutex
		workerCancelFunc       context.CancelFunc
		resetFunc              context.CancelFunc
		readRespBodyCancelFunc func()
		onStateChange          func()

		errMu          sync.RWMutex
		err            error
		status         WorkerStatus
		downloadStatus *transfer.DownloadStatus
	}

	// WorkerList worker 列表
	WorkerList []*Worker
)

// Duplicate 构造新的列表副本
func (wl WorkerList) Duplicate() WorkerList {
	n := make(WorkerList, len(wl))
	copy(n, wl)
	return n
}

// NewWorker 初始化 Worker
func NewWorker(id int, durl string, writerAt io.WriterAt) *Worker {
	return &Worker{
		id:         id,
		url:        durl,
		writerAt:   writerAt,
		speedsStat: &speeds.Speeds{},
		pauseChan:  make(chan struct{}),
	}
}

// ID 返回 worker ID
func (wer *Worker) ID() int {
	return wer.id
}

func (wer *Worker) setOnStateChange(fn func()) {
	wer.ctrlMu.Lock()
	wer.onStateChange = fn
	wer.ctrlMu.Unlock()
}

func (wer *Worker) setStatus(sc StatusCode) {
	wer.status.SetStatusCode(sc)
	switch sc {
	case StatusCodeSucceeded, StatusCodeCanceled, StatusCodeInternalError, StatusCodeFailed, StatusCodeNetError, StatusCodeTooManyConnections:
		wer.ctrlMu.Lock()
		fn := wer.onStateChange
		wer.ctrlMu.Unlock()
		if fn != nil {
			fn()
		}
	}
}

func (wer *Worker) setErr(err error) {
	wer.errMu.Lock()
	wer.err = err
	wer.errMu.Unlock()
}

func (wer *Worker) lazyInit() {
	wer.ctrlMu.Lock()
	defer wer.ctrlMu.Unlock()
	if wer.client == nil {
		wer.client = requester.NewHTTPClient()
	}
	if wer.pauseChan == nil {
		wer.pauseChan = make(chan struct{})
	}
	if wer.wrange == nil {
		wer.wrange = &transfer.Range{}
	}
	if wer.wrange.LoadBegin() == 0 && wer.wrange.LoadEnd() == 0 {
		// 取消多线程下载
		wer.acceptRanges = ""
		wer.wrange.StoreEnd(-2)
	}
	if wer.speedsStat == nil {
		wer.speedsStat = &speeds.Speeds{}
	}
}

// SetTotalSize 设置整个文件的大小
func (wer *Worker) SetTotalSize(size int64) {
	wer.totalSize = size
}

// SetClient 设置 http 客户端
func (wer *Worker) SetClient(c *requester.HTTPClient) {
	wer.client = c
}

// SetAcceptRange 设置 Accept-Ranges
func (wer *Worker) SetAcceptRange(acceptRanges string) {
	wer.acceptRanges = acceptRanges
}

// SetRange 设置请求范围
func (wer *Worker) SetRange(r *transfer.Range) {
	if wer.wrange == nil {
		wer.wrange = r
		return
	}
	wer.wrange.StoreBegin(r.LoadBegin())
	wer.wrange.StoreEnd(r.LoadEnd())
}

// SetReferer 设置来源
func (wer *Worker) SetReferer(referer string) {
	wer.referer = referer
}

// SetWriteMutex 设置数据写锁
func (wer *Worker) SetWriteMutex(mu *sync.Mutex) {
	wer.writeMu = mu
}

// SetDownloadStatus 关联总下载状态
func (wer *Worker) SetDownloadStatus(downloadStatus *transfer.DownloadStatus) {
	wer.downloadStatus = downloadStatus
}

// GetStatus 返回下载状态
func (wer *Worker) GetStatus() WorkerStatuser {
	return &wer.status
}

// GetRange 返回 worker 范围
func (wer *Worker) GetRange() *transfer.Range {
	return wer.wrange
}

// GetSpeedsPerSecond 获取每秒的速度
func (wer *Worker) GetSpeedsPerSecond() int64 {
	wer.ctrlMu.Lock()
	if wer.speedsStat == nil {
		wer.speedsStat = &speeds.Speeds{}
	}
	sp := wer.speedsStat
	wer.ctrlMu.Unlock()
	return sp.GetSpeeds()
}

// Pause 暂停下载
func (wer *Worker) Pause() {
	wer.lazyInit()
	if wer.acceptRanges == "" {
		pcsverbose.Verbosef("WARNING: worker unsupport pause")
		return
	}

	if wer.status.StatusCode() == StatusCodePaused {
		return
	}
	wer.pauseChan <- struct{}{}
	wer.setStatus(StatusCodePaused)
}

// Resume 恢复下载
func (wer *Worker) Resume() {
	if wer.status.StatusCode() != StatusCodePaused {
		return
	}
	go wer.Execute()
}

// Cancel 取消下载
func (wer *Worker) Cancel() error {
	wer.ctrlMu.Lock()
	cancelFn := wer.workerCancelFunc
	bodyCancelFn := wer.readRespBodyCancelFunc
	wer.ctrlMu.Unlock()
	if cancelFn == nil {
		return errors.New("cancelFunc not set")
	}
	cancelFn()
	if bodyCancelFn != nil {
		bodyCancelFn()
	}
	return nil
}

// Reset 重设连接
func (wer *Worker) Reset() {
	wer.ctrlMu.Lock()
	resetFn := wer.resetFunc
	bodyCancelFn := wer.readRespBodyCancelFunc
	wer.ctrlMu.Unlock()
	if resetFn == nil {
		pcsverbose.Verbosef("DEBUG: worker: resetFunc not set")
		return
	}
	resetFn()
	if bodyCancelFn != nil {
		bodyCancelFn()
	}
	wer.ClearStatus()
	go wer.Execute()
}

// Canceled 是否已经取消
func (wer *Worker) Canceled() bool {
	return wer.status.StatusCode() == StatusCodeCanceled
}

// Completed 是否已经完成
func (wer *Worker) Completed() bool {
	switch wer.status.StatusCode() {
	case StatusCodeSucceeded, StatusCodeCanceled:
		return true
	default:
		return false
	}
}

// Failed 是否失败
func (wer *Worker) Failed() bool {
	switch wer.status.StatusCode() {
	case StatusCodeFailed, StatusCodeInternalError, StatusCodeTooManyConnections, StatusCodeNetError:
		return true
	default:
		return false
	}
}

// ClearStatus 清空状态
func (wer *Worker) ClearStatus() {
	wer.setStatus(StatusCodeInit)
}

// Err 并发安全返回 worker 错误
func (wer *Worker) Err() error {
	wer.errMu.RLock()
	defer wer.errMu.RUnlock()
	return wer.err
}

// Execute 执行下载分片任务
func (wer *Worker) Execute() {
	wer.lazyInit()

	wer.execMu.Lock()
	defer wer.execMu.Unlock()

	if wer.status.StatusCode() == StatusCodePaused {
		return
	}
	wer.setStatus(StatusCodeInit)
	single := wer.acceptRanges == ""

	if !single {
		if rlen := wer.wrange.Len(); rlen <= 0 {
			if rlen < 0 {
				pcsverbose.Verbosef("DEBUG: RangeLen is negative at begin: %v, %d\n", wer.wrange, wer.wrange.Len())
			}
			wer.setStatus(StatusCodeSucceeded)
			return
		}
	}

	workerCancelCtx, workerCancelFunc := context.WithCancel(context.Background())
	resetCtx, resetFunc := context.WithCancel(context.Background())
	wer.ctrlMu.Lock()
	wer.workerCancelFunc = workerCancelFunc
	wer.resetFunc = resetFunc
	wer.ctrlMu.Unlock()
	defer func() {
		workerCancelFunc()
		resetFunc()
	}()

	header := map[string]string{}
	if wer.referer != "" {
		header["Referer"] = wer.referer
	}
	if wer.acceptRanges != "" && wer.wrange.Len() >= 0 {
		header["Range"] = fmt.Sprintf("%s=%d-%d", wer.acceptRanges, wer.wrange.LoadBegin(), wer.wrange.LoadEnd()-1)
	}

	wer.setStatus(StatusCodePending)

	var (
		resp   *http.Response
		reqErr error
	)
	if wer.firstResp != nil {
		resp = wer.firstResp
	} else {
		resp, reqErr = wer.client.Req(http.MethodGet, wer.url, nil, header)
		wer.setErr(reqErr)
	}
	if resp != nil {
		body := resp.Body
		wer.ctrlMu.Lock()
		wer.readRespBodyCancelFunc = func() {
			_ = body.Close()
		}
		wer.ctrlMu.Unlock()
		defer func() {
			wer.ctrlMu.Lock()
			wer.readRespBodyCancelFunc = nil
			wer.ctrlMu.Unlock()
			_ = body.Close()
			wer.firstResp = nil
		}()
	}
	if reqErr != nil {
		wer.setStatus(StatusCodeNetError)
		return
	}

	switch resp.StatusCode {
	case 200, 206:
		// ok
	case 416, 403, 404:
		wer.setErr(errors.New(resp.Status))
		wer.setStatus(StatusCodeInternalError)
		return
	case 406:
		wer.setErr(errors.New(resp.Status))
		wer.setStatus(StatusCodeNetError)
		return
	case 429, 509:
		wer.setErr(errors.New(resp.Status))
		wer.setStatus(StatusCodeTooManyConnections)
		return
	default:
		wer.setErr(fmt.Errorf("unexpected http status code, %d, %s", resp.StatusCode, resp.Status))
		wer.setStatus(StatusCodeNetError)
		return
	}

	var (
		contentLength = resp.ContentLength
		rangeLength   = wer.wrange.Len()
	)

	if !single {
		if contentLength != rangeLength && wer.firstResp == nil {
			wer.setErr(fmt.Errorf("Content-Length is unexpected: %d, need %d", contentLength, rangeLength))
			wer.setStatus(StatusCodeNetError)
			return
		}
		if wer.totalSize > 0 {
			total := ParseContentRange(resp.Header.Get("Content-Range"))
			if total > 0 && total != wer.totalSize {
				wer.setErr(fmt.Errorf("Content-Range total length is unexpected: %d, need %d", total, wer.totalSize))
				wer.setStatus(StatusCodeInternalError)
				return
			}
		}
	}

	var (
		buf       = acquireBuffer()
		n, nn     int
		n64, nn64 int64
	)
	defer releaseBuffer(buf)

	for {
		select {
		case <-workerCancelCtx.Done():
			wer.setStatus(StatusCodeCanceled)
			return
		case <-resetCtx.Done():
			wer.setStatus(StatusCodeReset)
			return
		case <-wer.pauseChan:
			return
		default:
			wer.setStatus(StatusCodeDownloading)

			var readErr error
			n = 0

			for n < len(buf) && readErr == nil && (single || wer.wrange.Len() > 0) {
				nn, readErr = resp.Body.Read(buf[n:])
				nn64 = int64(nn)

				if wer.downloadStatus != nil {
					wer.downloadStatus.AddSpeedsDownloaded(nn64)
				}
				wer.speedsStat.Add(nn64)
				n += nn
			}

			if n > 0 && readErr == io.EOF {
				readErr = io.ErrUnexpectedEOF
			}

			n64 = int64(n)

			if !single {
				rangeLength = wer.wrange.Len()
				if rangeLength <= 0 {
					wer.setErr(errors.New("worker already complete"))
					wer.setStatus(StatusCodeCanceled)
					return
				}

				if n64 > rangeLength {
					n64 = rangeLength
					n = int(rangeLength)
					readErr = io.EOF
				}
			}

			if wer.writerAt != nil {
				wer.setStatus(StatusCodeWaitToWrite)
				if wer.writeMu != nil {
					wer.writeMu.Lock()
				}
				_, writeErr := wer.writerAt.WriteAt(buf[:n], wer.wrange.LoadBegin())
				if wer.writeMu != nil {
					wer.writeMu.Unlock()
				}
				if writeErr != nil {
					wer.setErr(writeErr)
					wer.setStatus(StatusCodeInternalError)
					return
				}
				wer.setStatus(StatusCodeDownloading)
			}

			wer.wrange.AddBegin(n64)
			if wer.downloadStatus != nil {
				wer.downloadStatus.AddDownloaded(n64)
				if single {
					wer.downloadStatus.AddTotalSize(n64)
				}
			}

			if readErr != nil {
				rlen := wer.wrange.Len()
				switch {
				case (single && readErr == io.ErrUnexpectedEOF) || readErr == io.EOF || rlen <= 0:
					if rlen < 0 {
						pcsverbose.Verbosef("DEBUG: RangeLen is negative at end: %v, %d\n", wer.wrange, rlen)
					}
					wer.setStatus(StatusCodeSucceeded)
					return
				default:
					wer.setErr(readErr)
					wer.setStatus(StatusCodeFailed)
					return
				}
			}
		}
	}
}
