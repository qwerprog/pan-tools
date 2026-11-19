package downloader

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
)

var (
	// ErrNoWorkers 未提供任何下载 worker
	ErrNoWorkers = errors.New("no workers")
)

type (
	// Monitor 下载线程监控与协调器
	Monitor struct {
		initMu          sync.Mutex
		workersMu       sync.RWMutex
		workers         WorkerList
		status          *transfer.DownloadStatus
		instanceState   *InstanceState
		completed       chan struct{}
		completedOnce   sync.Once
		stateChanged    chan struct{}
		errMu           sync.RWMutex
		err             error
		resetController *ResetController
		isReloadWorker  bool

		lastAvaliableIndex int
	}

	// RangeWorkerFunc 遍历 workers 的回调函数
	RangeWorkerFunc func(key int, worker *Worker) bool
)

// NewMonitor 初始化 Monitor
func NewMonitor() *Monitor {
	return &Monitor{}
}

func (mt *Monitor) lazyInit() {
	mt.initMu.Lock()
	defer mt.initMu.Unlock()

	mt.workersMu.Lock()
	if mt.workers == nil {
		mt.workers = make(WorkerList, 0, 100)
	}
	mt.workersMu.Unlock()
	if mt.status == nil {
		mt.status = transfer.NewDownloadStatus()
	}
	if mt.resetController == nil {
		mt.resetController = NewResetController(80)
	}
	if mt.completed == nil {
		mt.completed = make(chan struct{})
	}
	if mt.stateChanged == nil {
		mt.stateChanged = make(chan struct{}, 1)
	}
}

func (mt *Monitor) closeCompleted() {
	mt.lazyInit()
	mt.completedOnce.Do(func() {
		close(mt.completed)
	})
}

func (mt *Monitor) snapshotWorkers() WorkerList {
	mt.workersMu.RLock()
	defer mt.workersMu.RUnlock()
	return mt.workers.Duplicate()
}

func (mt *Monitor) notifyStateChange() {
	if mt.stateChanged == nil {
		return
	}
	select {
	case mt.stateChanged <- struct{}{}:
	default:
	}
}

func (mt *Monitor) setErr(err error) {
	mt.errMu.Lock()
	mt.err = err
	mt.errMu.Unlock()
}

// InitMonitorCapacity 预分配 workers 容量
func (mt *Monitor) InitMonitorCapacity(capacity int) {
	mt.workersMu.Lock()
	mt.workers = make(WorkerList, 0, capacity)
	mt.workersMu.Unlock()
}

// Append 增加 Worker
func (mt *Monitor) Append(worker *Worker) {
	if worker == nil {
		return
	}
	mt.workersMu.Lock()
	mt.workers = append(mt.workers, worker)
	mt.workersMu.Unlock()
}

// SetWorkers 覆盖设置 workers
func (mt *Monitor) SetWorkers(workers WorkerList) {
	mt.workersMu.Lock()
	mt.workers = workers
	mt.workersMu.Unlock()
}

// SetStatus 设置 DownloadStatus
func (mt *Monitor) SetStatus(status *transfer.DownloadStatus) {
	mt.status = status
}

// SetInstanceState 设置断点状态存储
func (mt *Monitor) SetInstanceState(instanceState *InstanceState) {
	mt.instanceState = instanceState
}

// Status 返回 DownloadStatus
func (mt *Monitor) Status() *transfer.DownloadStatus {
	return mt.status
}

// Err 返回遇到的错误
func (mt *Monitor) Err() error {
	mt.errMu.RLock()
	defer mt.errMu.RUnlock()
	return mt.err
}

// CompletedChan 获取完成信号 channel
func (mt *Monitor) CompletedChan() <-chan struct{} {
	mt.lazyInit()
	return mt.completed
}

// SetReloadWorker 设置是否启用慢连接重载与分片复制
func (mt *Monitor) SetReloadWorker(b bool) {
	mt.isReloadWorker = b
}

// RangeWorker 遍历 worker
func (mt *Monitor) RangeWorker(f RangeWorkerFunc) {
	workers := mt.snapshotWorkers()
	for k := range workers {
		if !f(k, workers[k]) {
			break
		}
	}
}

// Pause 暂停所有的下载
func (mt *Monitor) Pause() {
	workers := mt.snapshotWorkers()
	for k := range workers {
		workers[k].Pause()
	}
}

// Resume 恢复所有的下载
func (mt *Monitor) Resume() {
	workers := mt.snapshotWorkers()
	for k := range workers {
		workers[k].Resume()
	}
}

// registerAllCompleted 事件驱动检测全部完成（辅以 200ms 兜底巡检，消除固定 1s 空等）
func (mt *Monitor) registerAllCompleted() {
	mt.lazyInit()

	checkDone := func() bool {
		workers := mt.snapshotWorkers()
		workerNum := len(workers)
		completeNum := 0
		for _, worker := range workers {
			switch worker.GetStatus().StatusCode() {
			case StatusCodeInternalError:
				mt.setErr(worker.Err())
				mt.closeCompleted()
				return true
			case StatusCodeSucceeded, StatusCodeCanceled:
				completeNum++
			}
		}
		gen := mt.status.RangeListGen()
		if completeNum >= workerNum && (gen == nil || gen.IsDone()) {
			mt.closeCompleted()
			return true
		}
		return false
	}

	go func() {
		if checkDone() {
			return
		}
		fallbackTicker := time.NewTicker(200 * time.Millisecond)
		defer fallbackTicker.Stop()
		for {
			select {
			case <-mt.completed:
				return
			case <-mt.stateChanged:
				if checkDone() {
					return
				}
			case <-fallbackTicker.C:
				if checkDone() {
					return
				}
			}
		}
	}()
}

// Execute 启动并监控所有下载工作单元
func (mt *Monitor) Execute(cancelCtx context.Context) {
	mt.lazyInit()
	initialWorkers := mt.snapshotWorkers()
	if len(initialWorkers) == 0 {
		mt.setErr(ErrNoWorkers)
		mt.closeCompleted()
		return
	}

	for _, worker := range initialWorkers {
		worker.lazyInit()
		worker.SetDownloadStatus(mt.status)
		worker.setOnStateChange(mt.notifyStateChange)
		go worker.Execute()
	}

	mt.registerAllCompleted()
	ticker := time.NewTicker(990 * time.Millisecond)
	ticker2 := time.NewTicker(99 * time.Millisecond)
	defer ticker.Stop()
	defer ticker2.Stop()

	for {
		select {
		case <-cancelCtx.Done():
			for _, worker := range mt.snapshotWorkers() {
				if err := worker.Cancel(); err != nil {
					pcsverbose.Verbosef("DEBUG: cancel failed, worker id: %d, err: %s\n", worker.ID(), err)
				}
			}
			mt.closeCompleted()
			return
		case <-mt.completed:
			return
		case <-ticker.C:
			mt.ResetFailedAndNetErrorWorkers()
			mt.status.UpdateSpeeds()

			if mt.instanceState != nil {
				mt.instanceState.Put(&transfer.DownloadInstanceInfo{
					DownloadStatus: mt.status,
					Ranges:         mt.GetAllWorkersRange(),
				})
			}

			if !mt.isReloadWorker {
				continue
			}

			mt.status.SetMaxSpeeds(mt.status.SpeedsPerSecond())

			isLeftWorkersAllFailed := mt.IsLeftWorkersAllFailed()
			if mt.status.SpeedsPerSecond() < mt.status.MaxSpeeds()/6 || isLeftWorkersAllFailed {
				if isLeftWorkersAllFailed {
					pcsverbose.Verbosef("DEBUG: monitor: All workers failed\n")
				}
				mt.status.ClearMaxSpeeds()

				pcsverbose.Verbosef("DEBUG: monitor: start duplicate.\n")
				sortedWorkers := mt.snapshotWorkers()
				sort.Sort(ByLeftDesc{sortedWorkers})
				for _, worker := range sortedWorkers {
					mt.DynamicSplitWorker(worker)
				}

				pcsverbose.Verbosef("DEBUG: monitor: start reload.\n")
				for _, worker := range sortedWorkers {
					mt.ResetWorker(worker)
				}
			}
		case <-ticker2.C:
			mt.TryAddNewWork()
		}
	}
}
