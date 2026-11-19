package pcsutil

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestWg(t *testing.T) {
	wg := NewWaitGroup(2)
	var count int32
	for i := 0; i < 10; i++ {
		wg.AddDelta()
		go func() {
			defer wg.Done()
			if wg.Parallel() > 2 {
				t.Errorf("parallel exceeded max: %d", wg.Parallel())
			}
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt32(&count, 1)
		}()
	}
	wg.Wait()
	if count != 10 {
		t.Fatalf("expected 10 tasks done, got %d", count)
	}
}
