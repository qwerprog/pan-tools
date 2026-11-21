package cachemap

import (
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/expires"
)

type (
	OpFunc func() expires.DataExpires
)

func (cm *CacheOpMap) CacheOperation(op string, key interface{}, opFunc OpFunc) (data expires.DataExpires) {
	var (
		cache = cm.LazyInitCachePoolOp(op)
		ok    bool
	)

	cache.LockKey(key)
	defer cache.UnlockKey(key)
	data, ok = cache.Load(key)
	if !ok {
		data = opFunc()
		if data != nil {
			cache.Store(key, data)
		}
		return
	}

	return
}
