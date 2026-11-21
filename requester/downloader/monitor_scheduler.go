package downloader

import (
	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
)

// GetAvailableWorker 获取空闲的 worker
func (mt *Monitor) GetAvailableWorker() *Worker {
	mt.workersMu.Lock()
	defer mt.workersMu.Unlock()
	workerCount := len(mt.workers)
	if workerCount == 0 {
		return nil
	}
	for i := mt.lastAvaliableIndex; i < mt.lastAvaliableIndex+workerCount; i++ {
		index := i % workerCount
		worker := mt.workers[index]
		if worker.Completed() {
			mt.lastAvaliableIndex = index
			return worker
		}
	}
	return nil
}

// GetAllWorkersRange 获取所有 worker 的范围快照
func (mt *Monitor) GetAllWorkersRange() transfer.RangeList {
	workers := mt.snapshotWorkers()
	allWorkerRanges := make(transfer.RangeList, 0, len(workers))
	for _, worker := range workers {
		allWorkerRanges = append(allWorkerRanges, worker.GetRange())
	}
	return allWorkerRanges
}

// IsLeftWorkersAllFailed 判断剩余未完成的线程是否全部处于失败状态
func (mt *Monitor) IsLeftWorkersAllFailed() bool {
	failedNum := 0
	for _, worker := range mt.snapshotWorkers() {
		if worker.Completed() {
			continue
		}
		if !worker.Failed() {
			return false
		}
		failedNum++
	}
	return failedNum != 0
}

// ResetFailedAndNetErrorWorkers 重设发生网络或下载错误的 worker
func (mt *Monitor) ResetFailedAndNetErrorWorkers() {
	workers := mt.snapshotWorkers()
	for k := range workers {
		if !mt.resetController.CanReset() {
			continue
		}

		sc := workers[k].GetStatus().StatusCode()
		if sc != StatusCodeNetError && sc != StatusCodeFailed {
			continue
		}
		pcsverbose.Verbosef("DEBUG: monitor: ResetFailedAndNetErrorWorkers: reset worker, id: %d, status: %v\n", workers[k].id, sc)
		workers[k].Reset()
		mt.resetController.AddResetNum()
	}
}

// TryAddNewWork 尝试给空闲 worker 派发新的分片 range
func (mt *Monitor) TryAddNewWork() {
	if mt.status == nil {
		return
	}
	gen := mt.status.RangeListGen()
	if gen == nil || gen.IsDone() {
		return
	}

	if !mt.resetController.CanReset() {
		return
	}

	availableWorker := mt.GetAvailableWorker()
	if availableWorker == nil {
		return
	}

	_, r := gen.GenRange()
	if r == nil {
		return
	}

	availableWorker.SetRange(r)
	availableWorker.ClearStatus()
	availableWorker.setOnStateChange(mt.notifyStateChange)

	mt.resetController.AddResetNum()
	pcsverbose.Verbosef("MONITOR: worker[%d] add new range: %s\n", availableWorker.ID(), r.ShowDetails())
	go availableWorker.Execute()
}

// DynamicSplitWorker 将剩余工作量较大的线程对半拆分给空闲线程
func (mt *Monitor) DynamicSplitWorker(worker *Worker) {
	if !mt.resetController.CanReset() {
		return
	}

	switch worker.GetStatus().StatusCode() {
	case StatusCodeDownloading, StatusCodeFailed, StatusCodeNetError:
	default:
		return
	}

	availableWorker := mt.GetAvailableWorker()
	if availableWorker == nil || worker == availableWorker {
		return
	}

	workerRange := worker.GetRange()
	end := workerRange.LoadEnd()
	middle := (workerRange.LoadBegin() + end) / 2

	if end-middle < MinParallelSize/5 {
		return
	}

	availableWorkerRange := availableWorker.GetRange()
	availableWorkerRange.StoreBegin(middle)
	availableWorkerRange.StoreEnd(end)
	availableWorker.ClearStatus()
	availableWorker.setOnStateChange(mt.notifyStateChange)

	workerRange.StoreEnd(middle)

	mt.resetController.AddResetNum()
	pcsverbose.Verbosef("MONITOR: worker duplicated: %d <- %d\n", availableWorker.ID(), worker.ID())
	go availableWorker.Execute()
}

// ResetWorker 重设长时间无响应且当前速度为 0 的 Worker
func (mt *Monitor) ResetWorker(worker *Worker) {
	if !mt.resetController.CanReset() || worker.Completed() {
		return
	}

	if worker.GetSpeedsPerSecond() != 0 {
		return
	}

	switch worker.GetStatus().StatusCode() {
	case StatusCodePending, StatusCodeReset, StatusCodeWaitToWrite, StatusCodePaused:
		return
	}

	mt.resetController.AddResetNum()
	pcsverbose.Verbosef("MONITOR: worker[%d] reload\n", worker.ID())
	worker.Reset()
}
