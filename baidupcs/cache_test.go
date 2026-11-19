package baidupcs

import (
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/expires"
)

func TestDeleteCacheInvalidatesDirectoryListings(t *testing.T) {
	pcs := NewPCS(266719, "dummy_bduss")
	pcs.lazyInit()
	cache := pcs.cacheOpMap.LazyInitCachePoolOp(OperationFilesDirectoriesList)

	defaultKey := listCacheKey("/work", DefaultOrderOptions)
	sizeDescKey := listCacheKey("/work", &OrderOptions{By: OrderBySize, Order: OrderDesc})
	otherDirKey := listCacheKey("/other", DefaultOrderOptions)

	cache.Store(defaultKey, expires.NewDataExpires(FileDirectoryList{}, time.Minute))
	cache.Store(sizeDescKey, expires.NewDataExpires(FileDirectoryList{}, time.Minute))
	cache.Store(otherDirKey, expires.NewDataExpires(FileDirectoryList{}, time.Minute))

	pcs.deleteCache([]string{"/work"})

	if _, ok := cache.Load(defaultKey); ok {
		t.Fatalf("expected default order cache entry %q to be invalidated", defaultKey)
	}
	if _, ok := cache.Load(sizeDescKey); ok {
		t.Fatalf("expected custom order cache entry %q to be invalidated", sizeDescKey)
	}
	if _, ok := cache.Load(otherDirKey); !ok {
		t.Fatalf("expected unrelated directory cache entry %q to remain", otherDirKey)
	}
}

func TestListCacheKeyNilOptionsFallback(t *testing.T) {
	got := listCacheKey("/docs", nil)
	want := listCacheKey("/docs", DefaultOrderOptions)
	if got != want {
		t.Fatalf("listCacheKey(/docs, nil) = %q, want %q", got, want)
	}
}

func TestDeleteCachePrefixIsolationAndPathCleanNormalization(t *testing.T) {
	pcs := NewPCS(266719, "dummy_bduss")
	pcs.lazyInit()
	cache := pcs.cacheOpMap.LazyInitCachePoolOp(OperationFilesDirectoriesList)

	// "/work/" and "/work/." must normalize to the exact same key as "/work"
	workSlashKey := listCacheKey("/work/", DefaultOrderOptions)
	workDotKey := listCacheKey("/work/.", DefaultOrderOptions)
	workCleanKey := listCacheKey("/work", DefaultOrderOptions)
	if workSlashKey != workCleanKey || workDotKey != workCleanKey {
		t.Fatalf("expected path.Clean key normalization: %q / %q / %q", workSlashKey, workDotKey, workCleanKey)
	}

	// "/work_2026" shares string prefix "/work_" with "/work", must NOT be evicted when "/work" is invalidated!
	work2026Key := listCacheKey("/work_2026", DefaultOrderOptions)
	cache.Store(workCleanKey, expires.NewDataExpires(FileDirectoryList{}, time.Minute))
	cache.Store(work2026Key, expires.NewDataExpires(FileDirectoryList{}, time.Minute))

	pcs.deleteCache([]string{"/work/"})

	if _, ok := cache.Load(workCleanKey); ok {
		t.Fatal("expected /work cache to be invalidated via /work/")
	}
	if _, ok := cache.Load(work2026Key); !ok {
		t.Fatal("/work_2026 cache was falsely evicted by prefix collision with /work")
	}
}
