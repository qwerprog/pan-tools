package downloader

import (
	"sync"
	"sync/atomic"

	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

var (
	defaultBufPoolSize atomic.Int64
	downloadBufPool    = sync.Pool{
		New: func() interface{} {
			size := int(defaultBufPoolSize.Load())
			if size <= 0 {
				size = int(64 * converter.KB)
			}
			buf := make([]byte, size)
			return &buf
		},
	}
)

func init() {
	defaultBufPoolSize.Store(64 * converter.KB)
}

func setBufferPoolSize(size int) {
	if size > 0 {
		defaultBufPoolSize.Store(int64(size))
	}
}

func acquireBuffer() []byte {
	want := int(defaultBufPoolSize.Load())
	if want <= 0 {
		want = int(64 * converter.KB)
	}
	if bp, ok := downloadBufPool.Get().(*[]byte); ok && bp != nil && cap(*bp) >= want {
		return (*bp)[:want]
	}
	return make([]byte, want)
}

func releaseBuffer(buf []byte) {
	if cap(buf) == 0 {
		return
	}
	b := buf[:cap(buf)]
	downloadBufPool.Put(&b)
}
