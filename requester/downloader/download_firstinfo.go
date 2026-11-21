package downloader

import (
	"net/http"
)

type (
	DownloadFirstInfo struct {
		ContentLength int64
		ContentMD5    string
		ContentCRC32  string
		AcceptRanges  string
		Referer       string
	}
)

func NewDownloadFirstInfoByResp(contentLength int64, resp *http.Response) (dfi *DownloadFirstInfo) {
	dfi = &DownloadFirstInfo{}
	if resp == nil {
		dfi.ContentLength = contentLength
		return
	}
	if contentLength >= 0 {
		dfi.ContentLength = contentLength
	} else {
		dfi.ContentLength = resp.ContentLength
	}
	dfi.AcceptRanges = resp.Header.Get("Accept-Ranges")
	dfi.Referer = resp.Header.Get("Referer")
	return
}
