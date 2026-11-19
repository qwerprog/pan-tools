package downloader

import (
	"sync"
	"time"
)

const resetWindow = 9 * time.Second

// ResetController 网络连接重设频率控制器
type ResetController struct {
	mu             sync.Mutex
	maxResetNum    int
	resetDeadlines []time.Time
}

// NewResetController 初始化 *ResetController
func NewResetController(maxResetNum int) *ResetController {
	return &ResetController{
		maxResetNum:    maxResetNum,
		resetDeadlines: make([]time.Time, 0, maxResetNum),
	}
}

func (rc *ResetController) pruneExpired(now time.Time) {
	kept := rc.resetDeadlines[:0]
	for _, dl := range rc.resetDeadlines {
		if now.Before(dl) {
			kept = append(kept, dl)
		}
	}
	rc.resetDeadlines = kept
}

// AddResetNum 记录一次连接重设
func (rc *ResetController) AddResetNum() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	now := time.Now()
	rc.pruneExpired(now)
	rc.resetDeadlines = append(rc.resetDeadlines, now.Add(resetWindow))
}

// CanReset 是否允许继续重设连接
func (rc *ResetController) CanReset() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.pruneExpired(time.Now())
	return len(rc.resetDeadlines) < rc.maxResetNum
}
