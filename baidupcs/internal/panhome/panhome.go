package panhome

import (
	"net/url"
	"sync"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/expires"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
)

const (
	// OperationSignature signature
	OperationSignature = "signature"
)

var (
	panBaiduComURL = &url.URL{
		Scheme: "https",
		Host:   "pan.baidu.com",
	}
	// PanHomeUserAgent PanHome User-Agent
	PanHomeUserAgent = "Mozilla/5.0"
)

type (
	PanHome struct {
		initMu sync.Mutex
		client *requester.HTTPClient

		sign1, sign3 []rune
		timestamp    string

		signMu      sync.Mutex
		signRes     SignRes
		signExpires expires.Expires
	}
)

func NewPanHome(client *requester.HTTPClient) *PanHome {
	ph := PanHome{}
	if client != nil {
		ph.client = client.Clone()
	}
	return &ph
}

func (ph *PanHome) lazyInit() {
	ph.initMu.Lock()
	defer ph.initMu.Unlock()
	if ph.client == nil {
		ph.client = requester.NewHTTPClient()
	}
}
