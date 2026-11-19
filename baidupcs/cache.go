package baidupcs

import (
	"path"
	"strings"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/expires"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
)

func normalizeCacheDir(dir string) string {
	cleaned := path.Clean(strings.TrimSpace(dir))
	if cleaned == "" || cleaned == "." {
		return PathSeparator
	}
	if !strings.HasPrefix(cleaned, PathSeparator) {
		cleaned = PathSeparator + cleaned
	}
	return cleaned
}

func listCacheKey(dir string, options *OrderOptions) string {
	if options == nil {
		options = DefaultOrderOptions
	}
	return normalizeCacheDir(dir) + "\x00" + string(options.By) + ":" + string(options.Order)
}

// deleteCache 删除含有 dirs 的缓存
func (pcs *BaiduPCS) deleteCache(dirs []string) {
	pcs.lazyInit()
	cache := pcs.cacheOpMap.LazyInitCachePoolOp(OperationFilesDirectoriesList)
	dirSet := make(map[string]struct{}, len(dirs))
	for _, v := range dirs {
		norm := normalizeCacheDir(v)
		dirSet[norm] = struct{}{}
		cache.Delete(listCacheKey(norm, DefaultOrderOptions))
	}
	// 同时失效同目录在其他排序条件下的缓存项（使用 NUL 分隔符杜绝 /work 与 /work_2026 前缀误删）
	cache.Range(func(k interface{}, _ expires.DataExpires) bool {
		ks, ok := k.(string)
		if !ok {
			return true
		}
		for d := range dirSet {
			if strings.HasPrefix(ks, d+"\x00") {
				cache.Delete(k)
				break
			}
		}
		return true
	})
}

// CacheFilesDirectoriesList 缓存获取
func (pcs *BaiduPCS) CacheFilesDirectoriesList(path string, options *OrderOptions) (fdl FileDirectoryList, pcsError pcserror.Error) {
	pcs.lazyInit()
	if options == nil {
		options = DefaultOrderOptions
	}
	data := pcs.cacheOpMap.CacheOperation(OperationFilesDirectoriesList, listCacheKey(path, options), func() expires.DataExpires {
		fdl, pcsError = pcs.FilesDirectoriesList(path, options)
		if pcsError != nil {
			return nil
		}
		return expires.NewDataExpires(fdl, 1*time.Minute)
	})
	if pcsError != nil {
		return
	}
	return data.Data().(FileDirectoryList), nil
}

// CacheUK 缓存获取
func (pcs *BaiduPCS) CacheUK() (uk int64, pcsError pcserror.Error) {
	pcs.lazyInit()
	data := pcs.cacheOpMap.CacheOperation(OperationGetUK, pcs.GetBDUSS(), func() expires.DataExpires {
		uk, pcsError = pcs.UK()
		if pcsError != nil {
			return nil
		}
		return expires.NewDataExpires(uk, 24*time.Hour)
	})
	if pcsError != nil {
		return
	}
	return data.Data().(int64), nil
}
