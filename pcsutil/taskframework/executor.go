package taskframework

import (
	"github.com/oleiade/lane"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type (
	TaskExecutor struct {
		incr     int32       // 任务id生成
		deque    *lane.Deque // 队列
		parallel int         // 任务的最大并发量
		stopped  int32       // 停止标志, 置位后不再派发新任务
		initMu   sync.Mutex
		stopChan chan struct{}
		stopOnce sync.Once

		// 是否统计失败队列
		IsFailedDeque bool
		failedDeque   *lane.Deque
	}
)

func NewTaskExecutor() *TaskExecutor {
	return &TaskExecutor{}
}

func (te *TaskExecutor) lazyInit() {
	te.initMu.Lock()
	defer te.initMu.Unlock()
	if te.deque == nil {
		te.deque = lane.NewDeque()
	}
	if te.parallel < 1 {
		te.parallel = 1
	}
	if te.stopChan == nil {
		te.stopChan = make(chan struct{})
		if atomic.LoadInt32(&te.stopped) == 1 {
			te.stopOnce.Do(func() {
				close(te.stopChan)
			})
		}
	}
	if te.IsFailedDeque && te.failedDeque == nil {
		te.failedDeque = lane.NewDeque()
	}
}

// SetParallel 设置任务的最大并发量
func (te *TaskExecutor) SetParallel(parallel int) {
	te.parallel = parallel
}

// Append 将任务加到任务队列末尾
func (te *TaskExecutor) Append(unit TaskUnit, maxRetry int) *TaskInfo {
	te.lazyInit()
	id := atomic.AddInt32(&te.incr, 1)
	taskInfo := &TaskInfo{
		id:       strconv.Itoa(int(id)),
		maxRetry: maxRetry,
	}
	unit.SetTaskInfo(taskInfo)
	te.deque.Append(&TaskInfoItem{
		Info: taskInfo,
		Unit: unit,
	})
	return taskInfo
}

// Execute 执行任务
func (te *TaskExecutor) Execute() {
	te.lazyInit()

	for {
		wg := pcsutil.NewWaitGroup(te.parallel)
		for {
			if te.Stopped() { // 已停止, 不再派发新任务
				break
			}

			e := te.deque.Shift()
			if e == nil { // 任务为空
				break
			}

			// 获取任务
			task := e.(*TaskInfoItem)
			wg.AddDelta()
			if te.Stopped() {
				if te.IsFailedDeque {
					te.failedDeque.Append(task)
				}
				wg.Done()
				break
			}

			go func(task *TaskInfoItem) {
				defer wg.Done()

				result := task.Unit.Run()

				// 返回结果为空
				if result == nil {
					task.Unit.OnComplete(result)
					return
				}

				if result.Succeed {
					task.Unit.OnSuccess(result)
					task.Unit.OnComplete(result)
					return
				}

				// 需要进行重试
				if result.NeedRetry {
					if te.Stopped() { // 已停止, 失败任务不再重试并记入失败队列
						task.Unit.OnFailed(result)
						if te.IsFailedDeque && result.Extra != "skip" {
							te.failedDeque.Append(task)
						}
						task.Unit.OnComplete(result)
						return
					}
					// 重试次数超出限制
					// 执行失败
					if task.Info.IsExceedRetry() {
						task.Unit.OnFailed(result)
						if te.IsFailedDeque {
							// 加入失败队列
							te.failedDeque.Append(task)
						}
						task.Unit.OnComplete(result)
						return
					}
					task.Info.retry++         // 增加重试次数
					task.Unit.OnRetry(result) // 调用重试
					task.Unit.OnComplete(result)

					waitDur := task.Unit.RetryWait()
					if waitDur > 0 {
						timer := time.NewTimer(waitDur)
						select {
						case <-timer.C:
						case <-te.stopChan:
							timer.Stop()
						}
					}
					if te.Stopped() {
						task.Unit.OnFailed(result)
						if te.IsFailedDeque && result.Extra != "skip" {
							te.failedDeque.Append(task)
						}
						return
					}
					te.deque.Append(task) // 重新加入队列末尾
					return
				}

				// 执行失败
				task.Unit.OnFailed(result)
				if te.IsFailedDeque && result.Extra != "skip" {
					// 加入失败队列
					te.failedDeque.Append(task)
				}
				task.Unit.OnComplete(result)
			}(task)
		}

		wg.Wait()

		if te.Stopped() { // 已停止, 将尚未启动的任务转入失败队列以便上层识别未完成状态
			if te.IsFailedDeque {
				for e := te.deque.Shift(); e != nil; e = te.deque.Shift() {
					te.failedDeque.Append(e)
				}
			}
			break
		}

		// 没有任务了
		if te.deque.Size() == 0 {
			break
		}
	}
}

// FailedDeque 获取失败队列
func (te *TaskExecutor) FailedDeque() *lane.Deque {
	return te.failedDeque
}

// Stop 停止执行: 不再派发新任务, 已在执行的任务继续运行直至完成
func (te *TaskExecutor) Stop() {
	atomic.StoreInt32(&te.stopped, 1)
	te.initMu.Lock()
	ch := te.stopChan
	te.initMu.Unlock()
	if ch != nil {
		te.stopOnce.Do(func() {
			close(ch)
		})
	}
}

// Stopped 返回是否已请求停止
func (te *TaskExecutor) Stopped() bool {
	return atomic.LoadInt32(&te.stopped) == 1
}
