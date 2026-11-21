package cachemap

import (
	"sync"
)

type (
	CacheOpMap struct {
		cachePool sync.Map
	}
)

func (cm *CacheOpMap) LazyInitCachePoolOp(op string) CacheUnit {
	cacheItf, _ := cm.cachePool.LoadOrStore(op, &cacheUnit{})
	return cacheItf.(CacheUnit)
}
