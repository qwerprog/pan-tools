package downloader

import (
	"net/http"
	"testing"
)

func TestNewDownloadFirstInfoByRespContentLength(t *testing.T) {
	resp := &http.Response{
		ContentLength: 4096,
		Header:        http.Header{"Accept-Ranges": []string{"bytes"}},
	}
	dfi := NewDownloadFirstInfoByResp(4096, resp)
	if dfi.ContentLength != 4096 {
		t.Fatalf("ContentLength = %d, want 4096 when matching resp.ContentLength", dfi.ContentLength)
	}

	dfiFallback := NewDownloadFirstInfoByResp(-1, resp)
	if dfiFallback.ContentLength != 4096 {
		t.Fatalf("ContentLength = %d, want 4096 fallback from resp.ContentLength", dfiFallback.ContentLength)
	}
}
