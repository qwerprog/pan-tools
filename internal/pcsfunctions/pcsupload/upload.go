package pcsupload

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/multipartreader"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/uploader"
)

var pcsPeriod = 256 // 上传多少个分片更换一次 pcsHost

type (
	PCSUpload struct {
		pcs        *baidupcs.BaiduPCS
		targetPath string
		client     *requester.HTTPClient
		clientOnce sync.Once
	}
)

func (pu *PCSUpload) getUploadClient(jar http.CookieJar) *requester.HTTPClient {
	pu.clientOnce.Do(func() {
		if pu.client == nil {
			pu.client = pcsconfig.Config.PCSHTTPClient()
		}
		pu.client.SetCookiejar(jar)
		pu.client.SetTimeout(200 * time.Second)
	})
	return pu.client
}

// NewPCSUploadWithClient 允许注入已配置好的 HTTPClient，避免传输层直接读取全局单例配置。
func NewPCSUploadWithClient(pcs *baidupcs.BaiduPCS, targetPath string, client *requester.HTTPClient) uploader.MultiUpload {
	return &PCSUpload{
		pcs:        pcs,
		targetPath: targetPath,
		client:     client,
	}
}

func (pu *PCSUpload) lazyInit() {
	if pu.pcs == nil {
		pu.pcs = &baidupcs.BaiduPCS{}
	}
}

// Precreate 上传前准备, 获取本次上传用的 pcs 服务器
func (pu *PCSUpload) Precreate() (originPCSHost string, err error) {
	pu.lazyInit()
	originPCSHost = pu.pcs.GetPCSAddr()
	newPCSHost, _ := pu.pcs.GetRandomPCSHost()
	if newPCSHost != "" {
		pu.pcs.SetPCSAddr(newPCSHost)
	}
	return originPCSHost, nil
}

func (pu *PCSUpload) TmpFile(ctx context.Context, uploadId, targetPath string, partSeq int, partOffset int64, r rio.ReaderLen64) (checksum string, uperr error) {
	pu.lazyInit()

	var respErr *uploader.MultiError

	if partSeq%pcsPeriod == pcsPeriod-1 {
		go func() {
			if newPCSHost, _ := pu.pcs.GetRandomPCSHost(); newPCSHost != "" {
				pu.pcs.SetPCSAddr(newPCSHost)
			}
		}()
	}

	checksum, pcsError := pu.pcs.UploadTmpFile(uploadId, targetPath, partSeq, partOffset, func(uploadURL string, jar http.CookieJar) (resp *http.Response, err error) {
		mr := multipartreader.NewMultipartReader()
		mr.AddFormFile("uploadedfile", "", r)
		mr.CloseMultipart()

		type reqResult struct {
			resp    *http.Response
			err     error
			respErr *uploader.MultiError
		}
		resChan := make(chan reqResult, 1)
		var canceled atomic.Bool

		go func() {
			cli := pu.getUploadClient(jar)
			rResp, rErr := cli.Req(http.MethodPost, uploadURL, mr, nil)
			if canceled.Load() {
				if rResp != nil && rResp.Body != nil {
					_ = rResp.Body.Close()
				}
				return
			}
			var mErr *uploader.MultiError
			if rResp != nil {
				switch rResp.StatusCode {
				case 400, 401, 403, 413:
					mErr = &uploader.MultiError{
						Terminated: true,
					}
				}
			}
			resChan <- reqResult{resp: rResp, err: rErr, respErr: mErr}
		}()

		select {
		case <-ctx.Done():
			canceled.Store(true)
			return nil, ctx.Err()
		case res := <-resChan:
			respErr = res.respErr
			return res.resp, res.err
		}
	})

	if respErr != nil {
		respErr.Err = pcsError
		return checksum, respErr
	}
	if pcsError != nil {
		return checksum, pcsError
	}
	return checksum, nil
}

func (pu *PCSUpload) CreateSuperFile(pcsHost, policy, uploadId string, fileSize int64, checksumMap map[int]string) error {
	pu.lazyInit()
	pu.pcs.SetPCSAddr(pcsHost)
	if pcsErr := pu.pcs.UploadCreateSuperFile(uploadId, policy, fileSize, pu.targetPath, checksumMap); pcsErr != nil {
		return pcsErr
	}
	return nil
}
