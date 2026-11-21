package requester

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"
)

type transportSelector struct {
	h *HTTPClient
}

func (ts transportSelector) RoundTrip(req *http.Request) (*http.Response, error) {
	return ts.h.currentTransport().RoundTrip(req)
}

func (ts transportSelector) CloseIdleConnections() {
	ts.h.CloseIdleConnections()
}

// HTTPClient http client
type HTTPClient struct {
	http.Client
	mu        sync.Mutex
	transport *http.Transport
	https     bool
	UserAgent string
}

// NewHTTPClient 返回 HTTPClient 的指针,
// 预设了一些配置
func NewHTTPClient() *HTTPClient {
	h := &HTTPClient{
		Client: http.Client{
			Timeout: 50 * time.Second,
		},
		UserAgent: UserAgent,
	}
	h.Client.Jar, _ = cookiejar.New(nil)
	return h
}

// Clone 复制一个共享底层 Transport/Jar 但拥有独立锁的 HTTPClient 指针。
func (h *HTTPClient) Clone() *HTTPClient {
	if h == nil {
		return nil
	}
	h.lazyInit()
	h.mu.Lock()
	defer h.mu.Unlock()
	cloned := &HTTPClient{
		Client:    h.Client,
		transport: h.transport,
		https:     h.https,
		UserAgent: h.UserAgent,
	}
	if ts, ok := h.Client.Transport.(transportSelector); ok && ts.h == h {
		cloned.Client.Transport = transportSelector{h: cloned}
	}
	return cloned
}

// Do 确保底层自定义 Transport 初始化后执行 HTTP 请求。
func (h *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	h.lazyInit()
	return h.Client.Do(req)
}

// Get 确保底层自定义 Transport 初始化后执行 GET 请求。
func (h *HTTPClient) Get(urlStr string) (*http.Response, error) {
	h.lazyInit()
	return h.Client.Get(urlStr)
}

// Head 确保底层自定义 Transport 初始化后执行 HEAD 请求。
func (h *HTTPClient) Head(urlStr string) (*http.Response, error) {
	h.lazyInit()
	return h.Client.Head(urlStr)
}

// Post 确保底层自定义 Transport 初始化后执行 POST 请求。
func (h *HTTPClient) Post(urlStr, contentType string, body io.Reader) (*http.Response, error) {
	h.lazyInit()
	return h.Client.Post(urlStr, contentType, body)
}

// PostForm 确保底层自定义 Transport 初始化后执行表单 POST 请求。
func (h *HTTPClient) PostForm(urlStr string, data url.Values) (*http.Response, error) {
	h.lazyInit()
	return h.Client.PostForm(urlStr, data)
}

// CloseIdleConnections 关闭底层 Transport 空闲连接池中的连接。
func (h *HTTPClient) CloseIdleConnections() {
	if h == nil {
		return
	}
	h.mu.Lock()
	t := h.transport
	h.mu.Unlock()
	if t != nil {
		t.CloseIdleConnections()
	}
}

func (h *HTTPClient) currentTransport() *http.Transport {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lazyInitLocked()
	return h.transport
}

func (h *HTTPClient) lazyInit() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lazyInitLocked()
}

func (h *HTTPClient) lazyInitLocked() {
	if h.transport == nil {
		h.transport = &http.Transport{
			Proxy:       proxyFunc,
			DialContext: dialContext,
			// DialTLS:     h.dialTLSFunc(),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !h.https,
			},
			TLSHandshakeTimeout:   20 * time.Second,
			DisableKeepAlives:     false,
			DisableCompression:    false, // gzip
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   60,
			ForceAttemptHTTP2:     true,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 25 * time.Second,
			ExpectContinueTimeout: 10 * time.Second,
		}
		if h.Client.Transport == nil {
			h.Client.Transport = transportSelector{h: h}
		}
	}
}

// SetUserAgent 设置 UserAgent 浏览器标识
func (h *HTTPClient) SetUserAgent(ua string) {
	h.UserAgent = ua
}

// SetProxy 设置代理
func (h *HTTPClient) SetProxy(proxyAddr string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lazyInitLocked()
	u, err := checkProxyAddr(proxyAddr)
	if err != nil {
		h.transport.Proxy = proxyFunc
		return
	}

	h.transport.Proxy = http.ProxyURL(u)
}

// SetCookiejar 设置 cookie
func (h *HTTPClient) SetCookiejar(jar http.CookieJar) {
	h.Client.Jar = jar
}

// ResetCookiejar 清空 cookie
func (h *HTTPClient) ResetCookiejar() {
	h.Jar, _ = cookiejar.New(nil)
}

// SetHTTPSecure 设置是否校验服务端 TLS 证书（默认跳过以兼容百度 CDN 动态节点）。
// 切换校验状态时通过 Transport.Clone() 派生新实例并关闭旧空闲连接，保留 ForceAttemptHTTP2 的 NextProtos ("h2", "http/1.1") 与自定义 RootCAs，且避免与并发请求发生竞态。
func (h *HTTPClient) SetHTTPSecure(verify bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.https = verify
	wasNil := h.transport == nil
	h.lazyInitLocked()
	wantSkip := !verify
	if h.transport.TLSClientConfig != nil && h.transport.TLSClientConfig.InsecureSkipVerify == wantSkip {
		return
	}
	if wasNil {
		if h.transport.TLSClientConfig == nil {
			h.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: wantSkip}
		} else {
			h.transport.TLSClientConfig.InsecureSkipVerify = wantSkip
		}
		return
	}
	oldTransport := h.transport
	clonedTransport := oldTransport.Clone()
	if clonedTransport.TLSClientConfig == nil {
		clonedTransport.TLSClientConfig = &tls.Config{}
	}
	clonedTransport.TLSClientConfig.InsecureSkipVerify = wantSkip
	h.transport = clonedTransport
	oldTransport.CloseIdleConnections()
}

// SetKeepAlive 设置 Keep-Alive
func (h *HTTPClient) SetKeepAlive(b bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lazyInitLocked()
	h.transport.DisableKeepAlives = !b
}

// SetResponseHeaderTimeout 设置目标服务器响应超时时间
func (h *HTTPClient) SetResponseHeaderTimeout(t time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lazyInitLocked()
	h.transport.ResponseHeaderTimeout = t
}

// SetTLSHandshakeTimeout 设置tls握手超时时间
func (h *HTTPClient) SetTLSHandshakeTimeout(t time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lazyInitLocked()
	h.transport.TLSHandshakeTimeout = t
}

// SetTimeout 设置 http 请求超时时间, 默认30s
func (h *HTTPClient) SetTimeout(t time.Duration) {
	h.Client.Timeout = t
}
