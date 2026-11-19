package getip

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/requester"
)

type rewriteRoundTripper struct {
	target *url.URL
	rt     http.RoundTripper
}

func (r rewriteRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = r.target.Scheme
	clone.URL.Host = r.target.Host
	return r.rt.RoundTrip(clone)
}

func TestIPInfoByClientMock(t *testing.T) {
	var bodyResp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, bodyResp)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cli := requester.NewHTTPClient()
	cli.Client.Transport = rewriteRoundTripper{target: u, rt: http.DefaultTransport}

	bodyResp = " 203.0.113.42 \n"
	ip, err := IPInfoByClient(cli)
	if err != nil || ip != "203.0.113.42" {
		t.Fatalf("IPInfoByClient valid IP got (%q, %v), want (\"203.0.113.42\", nil)", ip, err)
	}

	bodyResp = "not-an-ip-address"
	if _, err := IPInfoByClient(cli); !errors.Is(err, ErrParseIP) {
		t.Fatalf("expected ErrParseIP on invalid response, got %v", err)
	}
}
