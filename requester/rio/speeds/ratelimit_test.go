package speeds_test

import (
	"sync"
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio/speeds"
)

func TestRateLimitConcurrentAndStopUnblocks(t *testing.T) {
	r := speeds.NewRateLimit(100)
	r.SetInterval(20 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Add(60)
			r.SetInterval(15 * time.Millisecond)
			r.Add(60)
		}()
	}

	// Ensure Stop unblocks any goroutines waiting for next tick and is idempotent.
	time.Sleep(25 * time.Millisecond)
	r.Stop()
	r.Stop()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RateLimit goroutines deadlocked after Stop()")
	}
}
