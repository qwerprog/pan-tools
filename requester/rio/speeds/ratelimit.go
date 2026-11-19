package speeds

import (
	"sync"
	"sync/atomic"
	"time"
)

type (
	RateLimit struct {
		MaxRate int64

		mu              sync.Mutex
		count           int64
		interval        time.Duration
		ticker          *time.Ticker
		muChan          chan struct{}
		closeChan       chan struct{}
		stopped         bool
		backServiceOnce sync.Once
	}
)

func NewRateLimit(maxRate int64) *RateLimit {
	return &RateLimit{
		MaxRate: maxRate,
	}
}

func (rl *RateLimit) SetInterval(i time.Duration) {
	if i <= 0 {
		i = 1 * time.Second
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.interval = i
	if rl.ticker != nil && !rl.stopped {
		rl.ticker.Reset(i)
	}
}

func (rl *RateLimit) Stop() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.stopped {
		return
	}
	rl.stopped = true
	if rl.ticker != nil {
		rl.ticker.Stop()
	}
	if rl.closeChan != nil {
		close(rl.closeChan)
	}
	if rl.muChan != nil {
		close(rl.muChan)
		rl.muChan = nil
	}
}

func (rl *RateLimit) backService() {
	rl.mu.Lock()
	if rl.stopped {
		rl.mu.Unlock()
		return
	}
	if rl.interval <= 0 {
		rl.interval = 1 * time.Second
	}
	ticker := time.NewTicker(rl.interval)
	closeCh := make(chan struct{})
	rl.ticker = ticker
	rl.closeChan = closeCh
	rl.muChan = make(chan struct{})
	rl.mu.Unlock()

	go func() {
		for {
			select {
			case <-ticker.C:
				rl.mu.Lock()
				if rl.stopped {
					rl.mu.Unlock()
					return
				}
				atomic.StoreInt64(&rl.count, 0)
				if rl.muChan != nil {
					close(rl.muChan)
				}
				rl.muChan = make(chan struct{})
				rl.mu.Unlock()
			case <-closeCh:
				return
			}
		}
	}()
}

func (rl *RateLimit) Add(count int64) {
	rl.backServiceOnce.Do(rl.backService)
	for {
		maxRate := atomic.LoadInt64(&rl.MaxRate)
		if maxRate <= 0 {
			atomic.AddInt64(&rl.count, count)
			return
		}
		rl.mu.Lock()
		if rl.stopped {
			rl.mu.Unlock()
			return
		}
		if atomic.LoadInt64(&rl.count) < maxRate {
			atomic.AddInt64(&rl.count, count)
			rl.mu.Unlock()
			return
		}
		waitCh := rl.muChan
		rl.mu.Unlock()
		if waitCh == nil {
			return
		}
		<-waitCh
	}
}
