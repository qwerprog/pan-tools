package cachemap

import (
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/expires"
	"sync"
)

type (
	CacheUnit interface {
		Delete(key interface{})
		Load(key interface{}) (value expires.DataExpires, ok bool)
		LoadOrStore(key interface{}, value expires.DataExpires) (actual expires.DataExpires, loaded bool)
		Range(f func(key interface{}, value expires.DataExpires) bool)
		Store(key interface{}, value expires.DataExpires)
		LockKey(key interface{})
		UnlockKey(key interface{})
	}

	refLock struct {
		mu   sync.Mutex
		refs int
	}

	cacheUnit struct {
		unit    sync.Map
		locksMu sync.Mutex
		locks   map[interface{}]*refLock
	}
)

func (cu *cacheUnit) Delete(key interface{}) {
	cu.unit.Delete(key)
}

func (cu *cacheUnit) Load(key interface{}) (value expires.DataExpires, ok bool) {
	val, ok := cu.unit.Load(key)
	if !ok {
		return nil, ok
	}
	exp := val.(expires.DataExpires)
	if exp.IsExpires() {
		cu.unit.Delete(key)
		return nil, false
	}
	return exp, ok
}

func (cu *cacheUnit) Range(f func(key interface{}, value expires.DataExpires) bool) {
	cu.unit.Range(func(k, val interface{}) bool {
		exp := val.(expires.DataExpires)
		if exp.IsExpires() {
			cu.unit.Delete(k)
			return true
		}
		return f(k, val.(expires.DataExpires))
	})
}

func (cu *cacheUnit) LoadOrStore(key interface{}, value expires.DataExpires) (actual expires.DataExpires, loaded bool) {
	ac, loaded := cu.unit.LoadOrStore(key, value)
	exp := ac.(expires.DataExpires)
	if exp.IsExpires() {
		cu.unit.Delete(key)
		return nil, false
	}
	return exp, loaded
}

func (cu *cacheUnit) Store(key interface{}, value expires.DataExpires) {
	if value.IsExpires() {
		return
	}
	cu.unit.Store(key, value)
}

func (cu *cacheUnit) LockKey(key interface{}) {
	cu.locksMu.Lock()
	if cu.locks == nil {
		cu.locks = make(map[interface{}]*refLock)
	}
	rl := cu.locks[key]
	if rl == nil {
		rl = &refLock{}
		cu.locks[key] = rl
	}
	rl.refs++
	cu.locksMu.Unlock()

	rl.mu.Lock()
}

func (cu *cacheUnit) UnlockKey(key interface{}) {
	cu.locksMu.Lock()
	rl := cu.locks[key]
	if rl == nil || rl.refs <= 0 {
		cu.locksMu.Unlock()
		return
	}
	rl.refs--
	if rl.refs == 0 {
		delete(cu.locks, key)
	}
	cu.locksMu.Unlock()

	rl.mu.Unlock()
}
