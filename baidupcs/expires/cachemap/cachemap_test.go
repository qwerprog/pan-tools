package cachemap_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/expires"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/expires/cachemap"
)

func TestCacheMapDataExpires(t *testing.T) {
	cm := cachemap.CacheOpMap{}
	cache := cm.LazyInitCachePoolOp("op")
	cache.Store("key_1", expires.NewDataExpires("value_1", 30*time.Millisecond))

	if data, ok := cache.Load("key_1"); !ok || data.Data() != "value_1" {
		t.Fatalf("expected cached value_1 before TTL expiry, got (%v, %v)", data, ok)
	}

	time.Sleep(60 * time.Millisecond)
	if data, ok := cache.Load("key_1"); ok {
		t.Fatalf("expected cache entry to expire, still got %v", data.Data())
	}
}

func TestCacheOperation(t *testing.T) {
	cm := cachemap.CacheOpMap{}
	data := cm.CacheOperation("op", "key_1", func() expires.DataExpires {
		return expires.NewDataExpires("value_1", time.Second)
	})
	if data.Data() != "value_1" {
		t.Fatalf("got %v, want value_1", data.Data())
	}

	newData := cm.CacheOperation("op", "key_1", func() expires.DataExpires {
		return expires.NewDataExpires("value_3", time.Second)
	})
	if data != newData {
		t.Fatalf("expected cached DataExpires pointer hit, got %v vs %v", data, newData)
	}
}

func TestCacheOperation_LockKey(t *testing.T) {
	cm := cachemap.CacheOpMap{}
	var (
		wg         sync.WaitGroup
		execTimes1 atomic.Int32
		execTimes2 atomic.Int32
	)
	wg.Add(500)

	for i := 0; i < 500; i++ {
		go func(i int) {
			defer wg.Done()
			cm.CacheOperation("op", "key_1", func() expires.DataExpires {
				time.Sleep(50 * time.Microsecond)
				execTimes1.Add(1)
				return expires.NewDataExpires(fmt.Sprintf("value_1: %d", i), 10*time.Second)
			})
			cm.CacheOperation("op", "key_2", func() expires.DataExpires {
				time.Sleep(50 * time.Microsecond)
				execTimes2.Add(1)
				return expires.NewDataExpires(fmt.Sprintf("value_2: %d", i), 10*time.Second)
			})
		}(i)
	}
	wg.Wait()

	if execTimes1.Load() != 1 || execTimes2.Load() != 1 {
		t.Fatalf("expected single loader execution per key, got key_1=%d key_2=%d", execTimes1.Load(), execTimes2.Load())
	}
}

func TestCacheUnitConcurrentDeleteAndLock(t *testing.T) {
	cm := cachemap.CacheOpMap{}
	cu := cm.LazyInitCachePoolOp("concurrent_del")
	var wg sync.WaitGroup
	var active atomic.Int32

	for i := 0; i < 200; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			cu.LockKey("shared_key")
			if cur := active.Add(1); cur != 1 {
				t.Errorf("mutual exclusion violated: %d concurrent holders", cur)
			}
			time.Sleep(50 * time.Microsecond)
			active.Add(-1)
			cu.UnlockKey("shared_key")
		}()
		go func() {
			defer wg.Done()
			cu.Delete("shared_key")
		}()
	}
	wg.Wait()
	cu.UnlockKey("never_locked")
}
