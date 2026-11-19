package taskframework_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
)

type probeUnit struct {
	failUntilAttempt int32
	runs             atomic.Int32
	retries          atomic.Int32
	succeeded        atomic.Int32
	failed           atomic.Int32
	completed        atomic.Int32
	taskInfo         *taskframework.TaskInfo
}

func (u *probeUnit) SetTaskInfo(info *taskframework.TaskInfo) {
	u.taskInfo = info
}

func (u *probeUnit) Run() *taskframework.TaskUnitRunResult {
	attempt := u.runs.Add(1)
	if attempt <= u.failUntilAttempt {
		return &taskframework.TaskUnitRunResult{
			NeedRetry: true,
			Err:       errors.New("transient error"),
		}
	}
	return &taskframework.TaskUnitRunResult{Succeed: true}
}

func (u *probeUnit) OnRetry(*taskframework.TaskUnitRunResult)    { u.retries.Add(1) }
func (u *probeUnit) OnSuccess(*taskframework.TaskUnitRunResult)  { u.succeeded.Add(1) }
func (u *probeUnit) OnFailed(*taskframework.TaskUnitRunResult)   { u.failed.Add(1) }
func (u *probeUnit) OnComplete(*taskframework.TaskUnitRunResult) { u.completed.Add(1) }
func (u *probeUnit) RetryWait() time.Duration                    { return 5 * time.Millisecond }

func TestTaskExecutorRetriesAndFailureTracking(t *testing.T) {
	te := taskframework.NewTaskExecutor()
	te.IsFailedDeque = true
	te.SetParallel(2)

	// u1: 首次失败，第 2 次重试成功 (maxRetry=2)
	u1 := &probeUnit{failUntilAttempt: 1}
	// u2: 持续失败超过 maxRetry=2 (共运行 1 + 2 = 3 次，最终失败)
	u2 := &probeUnit{failUntilAttempt: 99}

	te.Append(u1, 2)
	te.Append(u2, 2)
	te.Execute()

	if u1.runs.Load() != 2 || u1.retries.Load() != 1 || u1.succeeded.Load() != 1 || u1.failed.Load() != 0 || u1.completed.Load() != 2 {
		t.Fatalf("unexpected u1 lifecycle counts: runs=%d retries=%d succ=%d fail=%d comp=%d",
			u1.runs.Load(), u1.retries.Load(), u1.succeeded.Load(), u1.failed.Load(), u1.completed.Load())
	}

	if u2.runs.Load() != 3 || u2.retries.Load() != 2 || u2.succeeded.Load() != 0 || u2.failed.Load() != 1 || u2.completed.Load() != 3 {
		t.Fatalf("unexpected u2 lifecycle counts: runs=%d retries=%d succ=%d fail=%d comp=%d",
			u2.runs.Load(), u2.retries.Load(), u2.succeeded.Load(), u2.failed.Load(), u2.completed.Load())
	}

	if got := te.FailedDeque().Size(); got != 1 {
		t.Fatalf("FailedDeque().Size() = %d, want 1", got)
	}
}

type stopAwareUnit struct {
	onRun  func() *taskframework.TaskUnitRunResult
	failed atomic.Int32
}

func (u *stopAwareUnit) SetTaskInfo(*taskframework.TaskInfo)         {}
func (u *stopAwareUnit) Run() *taskframework.TaskUnitRunResult       { return u.onRun() }
func (u *stopAwareUnit) OnRetry(*taskframework.TaskUnitRunResult)    {}
func (u *stopAwareUnit) OnSuccess(*taskframework.TaskUnitRunResult)  {}
func (u *stopAwareUnit) OnFailed(*taskframework.TaskUnitRunResult)   { u.failed.Add(1) }
func (u *stopAwareUnit) OnComplete(*taskframework.TaskUnitRunResult) {}
func (u *stopAwareUnit) RetryWait() time.Duration                    { return time.Millisecond }

func TestTaskExecutorStopTracksUnstartedAndRetryingTasksInFailedDeque(t *testing.T) {
	te := taskframework.NewTaskExecutor()
	te.IsFailedDeque = true
	te.SetParallel(1)

	// First task triggers Stop() and returns NeedRetry=true; tasks 2 and 3 remain unstarted in deque.
	u1 := &stopAwareUnit{
		onRun: func() *taskframework.TaskUnitRunResult {
			te.Stop()
			return &taskframework.TaskUnitRunResult{NeedRetry: true, Err: errors.New("interrupted")}
		},
	}
	u2 := &stopAwareUnit{onRun: func() *taskframework.TaskUnitRunResult {
		t.Error("u2 must not start after Stop()")
		return &taskframework.TaskUnitRunResult{Succeed: true}
	}}
	u3 := &stopAwareUnit{onRun: func() *taskframework.TaskUnitRunResult {
		t.Error("u3 must not start after Stop()")
		return &taskframework.TaskUnitRunResult{Succeed: true}
	}}

	te.Append(u1, 3)
	te.Append(u2, 3)
	te.Append(u3, 3)
	te.Execute()

	if !te.Stopped() {
		t.Fatal("expected executor.Stopped() == true")
	}
	if u1.failed.Load() != 1 {
		t.Fatalf("expected retrying task u1 OnFailed called once on Stop, got %d", u1.failed.Load())
	}
	if got := te.FailedDeque().Size(); got != 3 {
		t.Fatalf("FailedDeque().Size() after Stop = %d, want 3 (1 retrying + 2 unstarted)", got)
	}

	// Stop() during a long RetryWait() must wake up immediately instead of blocking on time.Sleep.
	teSlow := taskframework.NewTaskExecutor()
	teSlow.IsFailedDeque = true
	inRetry := make(chan struct{})
	slowRetryUnit := &customWaitUnit{
		wait: 5 * time.Second,
		onRetry: func() {
			close(inRetry)
		},
	}
	teSlow.Append(slowRetryUnit, 3)

	done := make(chan struct{})
	go func() {
		defer close(done)
		teSlow.Execute()
	}()
	<-inRetry
	startStop := time.Now()
	teSlow.Stop()
	<-done
	if elapsed := time.Since(startStop); elapsed > 500*time.Millisecond {
		t.Fatalf("Stop() blocked on RetryWait sleep for %v", elapsed)
	}
	if slowRetryUnit.failed.Load() != 1 || teSlow.FailedDeque().Size() != 1 {
		t.Fatalf("expected OnFailed=1 and FailedDeque=1 on interrupted retry wait, got failed=%d deque=%d",
			slowRetryUnit.failed.Load(), teSlow.FailedDeque().Size())
	}
}

type customWaitUnit struct {
	wait    time.Duration
	onRetry func()
	failed  atomic.Int32
}

func (u *customWaitUnit) SetTaskInfo(*taskframework.TaskInfo) {}
func (u *customWaitUnit) Run() *taskframework.TaskUnitRunResult {
	return &taskframework.TaskUnitRunResult{NeedRetry: true, Err: errors.New("retry me")}
}
func (u *customWaitUnit) OnRetry(*taskframework.TaskUnitRunResult) {
	if u.onRetry != nil {
		u.onRetry()
	}
}
func (u *customWaitUnit) OnSuccess(*taskframework.TaskUnitRunResult)  {}
func (u *customWaitUnit) OnFailed(*taskframework.TaskUnitRunResult)   { u.failed.Add(1) }
func (u *customWaitUnit) OnComplete(*taskframework.TaskUnitRunResult) {}
func (u *customWaitUnit) RetryWait() time.Duration                    { return u.wait }
