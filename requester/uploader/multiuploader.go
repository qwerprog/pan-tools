package uploader

import (
	"context"
	"errors"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio/speeds"
	"sync"
	"time"
)

var (
	uploaderVerbose = pcsverbose.New("UPLOADER")
)

type (
	// MultiUpload 支持多线程的上传, 可用于断点续传
	MultiUpload interface {
		Precreate() (pcsHost string, err error)
		TmpFile(ctx context.Context, uploadid, targetPath string, partseq int, partOffset int64, readerlen64 rio.ReaderLen64) (checksum string, terr error)
		CreateSuperFile(pcsHost, policy, uploadId string, fileSize int64, checksumMap map[int]string) (cerr error)
	}

	// MultiUploader 多线程上传
	MultiUploader struct {
		onSuccessEvent      requester.Event        //成功上传事件
		onFinishEvent       requester.Event        //结束上传事件
		onCancelEvent       requester.Event        //取消上传事件
		onErrorEvent        requester.EventOnError //上传出错事件
		onUploadStatusEvent UploadStatusFunc       //上传状态事件

		stateMu       sync.RWMutex
		instanceState *InstanceState

		multiUpload MultiUpload       // 上传体接口
		file        rio.ReaderAtLen64 // 上传
		config      *MultiUploaderConfig
		workers     workerList
		speedsStat  *speeds.Speeds
		rateLimit   *speeds.RateLimit
		targetPath  string

		executeTime             time.Time
		finished                chan struct{}
		canceled                chan struct{}
		closeCanceledOnce       sync.Once
		stopped                 chan struct{} // 优雅停止信号: 不再派发新分片, 进行中的分片继续完成
		stopOnce                sync.Once
		updateInstanceStateChan chan struct{}
	}

	// MultiUploaderConfig 多线程上传配置
	MultiUploaderConfig struct {
		Parallel  int    // 上传并发量
		BlockSize int64  // 上传分块
		MaxRate   int64  // 限制最大上传速度
		Policy    string // 文件重名策略
	}
)

// NewMultiUploader 初始化上传
func NewMultiUploader(multiUpload MultiUpload, file rio.ReaderAtLen64, config *MultiUploaderConfig, targetPath string) *MultiUploader {
	return &MultiUploader{
		multiUpload: multiUpload,
		file:        file,
		config:      config,
		targetPath:  targetPath,

		finished:                make(chan struct{}, 1),
		canceled:                make(chan struct{}),
		stopped:                 make(chan struct{}),
		updateInstanceStateChan: make(chan struct{}, 1),
	}
}

// SetInstanceState 设置InstanceState, 断点续传信息
func (muer *MultiUploader) SetInstanceState(is *InstanceState) {
	muer.stateMu.Lock()
	muer.instanceState = is
	muer.stateMu.Unlock()
}

func (muer *MultiUploader) getUploadID() string {
	muer.stateMu.RLock()
	defer muer.stateMu.RUnlock()
	if muer.instanceState != nil {
		return muer.instanceState.Uploadid
	}
	return ""
}

func (muer *MultiUploader) lazyInit() {
	if muer.finished == nil {
		muer.finished = make(chan struct{}, 1)
	}
	if muer.canceled == nil {
		muer.canceled = make(chan struct{})
	}
	if muer.stopped == nil {
		muer.stopped = make(chan struct{})
	}
	if muer.updateInstanceStateChan == nil {
		muer.updateInstanceStateChan = make(chan struct{}, 1)
	}
	if muer.config == nil {
		muer.config = &MultiUploaderConfig{}
	}
	if muer.config.Parallel <= 0 {
		muer.config.Parallel = 4
	}
	if muer.config.BlockSize <= 0 {
		muer.config.BlockSize = 16 * converter.MB
	}
	if muer.speedsStat == nil {
		muer.speedsStat = &speeds.Speeds{}
	}
	muer.stateMu.Lock()
	if muer.instanceState == nil {
		muer.instanceState = &InstanceState{}
	}
	muer.stateMu.Unlock()
}

func (muer *MultiUploader) check() error {
	if muer.file == nil {
		return errors.New("uploader: file is nil")
	}
	if muer.multiUpload == nil {
		return errors.New("uploader: multiUpload is nil")
	}
	return nil
}

// Execute 执行上传
func (muer *MultiUploader) Execute() {
	muer.lazyInit()
	if err := muer.check(); err != nil {
		select {
		case muer.finished <- struct{}{}:
		default:
		}
		if muer.onErrorEvent != nil {
			muer.onErrorEvent(err)
		}
		pcsutil.TriggerOnSync(muer.onFinishEvent)
		return
	}
	// 初始化限速
	if muer.config.MaxRate > 0 {
		muer.rateLimit = speeds.NewRateLimit(muer.config.MaxRate)
		defer muer.rateLimit.Stop()
	}

	// 分配任务
	muer.stateMu.RLock()
	initState := muer.instanceState
	muer.stateMu.RUnlock()
	var builtWorkers workerList
	if initState != nil && len(initState.BlockList) > 0 {
		builtWorkers = muer.getWorkerListByInstanceState(initState)
		uploaderVerbose.Infof("upload task CREATED from instance state\n")
	} else {
		builtWorkers = muer.getWorkerListByInstanceState(&InstanceState{
			BlockList: SplitBlock(muer.file.Len(), muer.config.BlockSize),
		})
		uploaderVerbose.Infof("upload task CREATED: block size: %d, num: %d\n", muer.config.BlockSize, len(builtWorkers))
	}
	muer.stateMu.Lock()
	muer.workers = builtWorkers
	muer.stateMu.Unlock()

	// 开始上传
	muer.executeTime = time.Now()

	muer.uploadStatusEvent()

	err := muer.upload()

	// 完成
	muer.finished <- struct{}{}
	if err != nil {
		if err == context.Canceled {
			if muer.onCancelEvent != nil {
				muer.onCancelEvent()
			}
		} else if muer.onErrorEvent != nil {
			muer.onErrorEvent(err)
		}
	} else {
		pcsutil.TriggerOnSync(muer.onSuccessEvent)
	}
	pcsutil.TriggerOnSync(muer.onFinishEvent)
}

// InstanceState 返回断点续传信息
func (muer *MultiUploader) InstanceState() *InstanceState {
	muer.stateMu.RLock()
	workers := muer.workers
	var uploadID string
	if muer.instanceState != nil {
		uploadID = muer.instanceState.Uploadid
	}
	muer.stateMu.RUnlock()

	blockStates := make([]*BlockState, 0, len(workers))
	for _, wer := range workers {
		blockStates = append(blockStates, &BlockState{
			ID:       wer.id,
			Range:    wer.splitUnit.Range(),
			CheckSum: wer.getChecksum(),
		})
	}
	return &InstanceState{
		BlockList: blockStates,
		Uploadid:  uploadID,
	}
}

// Cancel 取消上传
func (muer *MultiUploader) Cancel() {
	muer.closeCanceledOnce.Do(func() {
		close(muer.canceled)
	})
}

// GracefulStop 优雅停止: 不再派发新分片, 进行中的分片继续执行直至完成
func (muer *MultiUploader) GracefulStop() {
	muer.stopOnce.Do(func() {
		close(muer.stopped)
	})
}

// isStopped 返回是否已请求优雅停止
func (muer *MultiUploader) isStopped() bool {
	select {
	case <-muer.stopped:
		return true
	default:
		return false
	}
}

// OnSuccess 设置成功上传事件
func (muer *MultiUploader) OnSuccess(onSuccessEvent requester.Event) {
	muer.onSuccessEvent = onSuccessEvent
}

// OnFinish 设置结束上传事件
func (muer *MultiUploader) OnFinish(onFinishEvent requester.Event) {
	muer.onFinishEvent = onFinishEvent
}

// OnCancel 设置取消上传事件
func (muer *MultiUploader) OnCancel(onCancelEvent requester.Event) {
	muer.onCancelEvent = onCancelEvent
}

// OnError 设置上传发生错误事件
func (muer *MultiUploader) OnError(onErrorEvent requester.EventOnError) {
	muer.onErrorEvent = onErrorEvent
}

// OnUploadStatusEvent 设置上传状态事件
func (muer *MultiUploader) OnUploadStatusEvent(f UploadStatusFunc) {
	muer.onUploadStatusEvent = f
}
