package requester

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTCPHostCacheTTLAndPermanentBind(t *testing.T) {
	c := newTCPHostCache()
	var calls atomic.Int32

	ip1 := net.ParseIP("198.51.100.10")
	ip2 := net.ParseIP("198.51.100.20")
	permIP := net.ParseIP("203.0.113.99")

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := c.getOrLoad("pcs.baidu.com", 40*time.Millisecond, func() (net.IP, error) {
				calls.Add(1)
				time.Sleep(5 * time.Millisecond)
				return ip1, nil
			})
			if err != nil || !got.Equal(ip1) {
				t.Errorf("got (%v, %v), want (%v, nil)", got, err, ip1)
			}
		}()
	}
	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("expected singleflight to deduplicate concurrent loads to 1 call, got %d", calls.Load())
	}

	time.Sleep(55 * time.Millisecond)
	gotAfterTTL, err := c.getOrLoad("pcs.baidu.com", time.Minute, func() (net.IP, error) {
		calls.Add(1)
		return ip2, nil
	})
	if err != nil || !gotAfterTTL.Equal(ip2) {
		t.Fatalf("after TTL expiry got (%v, %v), want (%v, nil)", gotAfterTTL, err, ip2)
	}

	c.storePermanent("pcs.baidu.com", permIP)
	gotPerm, err := c.getOrLoad("pcs.baidu.com", time.Nanosecond, func() (net.IP, error) {
		t.Fatal("loader must not run for permanently bound host")
		return nil, nil
	})
	if err != nil || !gotPerm.Equal(permIP) {
		t.Fatalf("permanent host bind got (%v, %v), want (%v, nil)", gotPerm, err, permIP)
	}
}

func TestDialContextHonorsCancellationAndHTTPClientConcurrentLazyInit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	conn, err := dialContext(ctx, "tcp", "127.0.0.1:65534")
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("expected pre-canceled dialContext to fail immediately")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("dialContext ignored canceled context, took %v", elapsed)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	hc := NewHTTPClient()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%3 == 0 {
				_ = hc.Clone()
			}
			if idx%5 == 0 {
				SetLocalTCPAddrList()
				SetGlobalProxy("")
				SetProxyHostnameRules("")
			}
			body, fetchErr := hc.Fetch(http.MethodGet, srv.URL, nil, nil)
			if fetchErr != nil || string(body) != "ok" {
				t.Errorf("concurrent Fetch failed: body=%q err=%v", string(body), fetchErr)
			}
		}(i)
	}
	wg.Wait()

	// Empty localTCPAddrList must keep Dialer.LocalAddr == untyped nil so non-TCP dials (e.g. UDP) succeed.
	SetLocalTCPAddrList()
	if got := getDialer().LocalAddr; got != nil {
		t.Fatalf("expected untyped nil LocalAddr when localTCPAddrList is empty, got %#v", got)
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket udp failed: %v", err)
	}
	defer pc.Close()
	udpConn, err := dialContext(context.Background(), "udp", pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("dialContext udp failed with typed-nil LocalAddr bug: %v", err)
	}
	_ = udpConn.Close()
}

func TestSetHTTPSecurePreservesHTTP2NextProtosAndVerifiesCert(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Proto", r.Proto)
		_, _ = w.Write([]byte(r.Proto))
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	hc := NewHTTPClient()
	defer hc.CloseIdleConnections()

	// 1. Initial insecure request negotiates HTTP/2 and populates Transport's TLSClientConfig.NextProtos.
	hc.SetHTTPSecure(false)
	req1, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp1, err := hc.Do(req1)
	if err != nil {
		t.Fatalf("initial SetHTTPSecure(false) request failed: %v", err)
	}
	_ = resp1.Body.Close()
	if resp1.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2 on initial request, got %s", resp1.Proto)
	}

	// 2. Enabling TLS verification afterwards must NOT wipe NextProtos ("h2", "http/1.1"),
	//    and must flush idle unverified connections so untrusted self-signed cert is rejected.
	hc.SetHTTPSecure(true)
	hc.mu.Lock()
	nextProtos := append([]string(nil), hc.transport.TLSClientConfig.NextProtos...)
	skipVerify := hc.transport.TLSClientConfig.InsecureSkipVerify
	hc.mu.Unlock()

	if skipVerify {
		t.Fatal("expected InsecureSkipVerify=false after SetHTTPSecure(true)")
	}
	if !slices.Contains(nextProtos, "h2") || !slices.Contains(nextProtos, "http/1.1") {
		t.Fatalf("SetHTTPSecure(true) wiped HTTP/2 ALPN NextProtos: got %v", nextProtos)
	}

	req2, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	if resp2, err := hc.Do(req2); err == nil {
		_ = resp2.Body.Close()
		t.Fatal("expected SetHTTPSecure(true) to reject untrusted self-signed TLS server without reusing insecure idle conn")
	}

	// 3. Injecting the server RootCA and calling SetHTTPSecure(true) again preserves both RootCAs and HTTP/2 ALPN.
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	hc.mu.Lock()
	cfg := hc.transport.TLSClientConfig.Clone()
	cfg.RootCAs = pool
	hc.transport.TLSClientConfig = cfg
	hc.mu.Unlock()
	hc.SetHTTPSecure(true)

	req3, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp3, err := hc.Do(req3)
	if err != nil {
		t.Fatalf("trusted SetHTTPSecure(true) request failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2 preserved after SetHTTPSecure(true), got %s", resp3.Proto)
	}
	if resp3.TLS == nil || resp3.TLS.Version < tls.VersionTLS12 {
		t.Fatalf("unexpected TLS state: %#v", resp3.TLS)
	}
}

func TestDialContextMultiIPFallbackPromotionAndCacheRotation(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen tcp4 127.0.0.1:0 failed: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			c, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			_ = c.Close()
			select {
			case <-done:
				return
			default:
			}
		}
	}()

	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	const testHost = "passport-multi-ip-test.baidu.internal"
	defer SetTCPHostBind(testHost, "")

	var lookupCalls atomic.Int32
	dialConfigMu.Lock()
	origLookup := dnsLookupHostFunc
	dnsLookupHostFunc = func(_ context.Context, host string) ([]string, error) {
		if strings.EqualFold(host, testHost) {
			n := lookupCalls.Add(1)
			if n == 1 {
				// First IPv4 (127.0.0.2) and IPv6 (::1) have no listener on portStr; third IP (127.0.0.1) is live!
				return []string{"::1", "127.0.0.2", "127.0.0.1"}, nil
			}
			return []string{"127.0.0.1"}, nil
		}
		return origLookup(context.Background(), host)
	}
	dialConfigMu.Unlock()
	defer func() {
		dialConfigMu.Lock()
		dnsLookupHostFunc = origLookup
		dialConfigMu.Unlock()
		InvalidateTCPHostCache(testHost)
	}()

	InvalidateTCPHostCache(testHost)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := dialContext(ctx, "tcp", net.JoinHostPort(testHost, portStr))
	if err != nil {
		t.Fatalf("dialContext failed to fall back across dead IPs to live 127.0.0.1: %v", err)
	}
	_ = conn.Close()

	if lookupCalls.Load() != 1 {
		t.Fatalf("expected 1 DNS lookup, got %d", lookupCalls.Load())
	}

	// Verify 127.0.0.1 was promoted to index 0 in tcpCache after fallback succeeded.
	cachedAfterPromote, err := tcpCache.getOrLoadIPs(ctx, testHost, time.Minute, func(context.Context) ([]net.IP, error) {
		t.Fatal("expected cached entry")
		return nil, nil
	})
	if err != nil || len(cachedAfterPromote) != 3 || !cachedAfterPromote[0].Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("expected 127.0.0.1 promoted to index 0, got %v (err=%v)", cachedAfterPromote, err)
	}

	// Rotating 3 times on a 3-IP cache entry must cycle through remaining candidates and then evict on exhaustion.
	RotateTCPHostCache(testHost) // rotated=1 -> [127.0.0.2, ::1, 127.0.0.1]
	RotateTCPHostCache(testHost) // rotated=2 -> [::1, 127.0.0.1, 127.0.0.2]
	RotateTCPHostCache(testHost) // rotated+1 >= 3 -> evicted!

	conn2, err := dialContext(ctx, "tcp4", net.JoinHostPort(testHost, portStr))
	if err != nil {
		t.Fatalf("dialContext after full cache rotation eviction failed: %v", err)
	}
	_ = conn2.Close()
	if lookupCalls.Load() != 2 {
		t.Fatalf("expected fresh DNS lookup (#2) after exhausting rotated IPs, got %d", lookupCalls.Load())
	}

	// Permanent host bind must ignore RotateTCPHostCache and InvalidateTCPHostCache, and empty string unbinds cleanly.
	SetTCPHostBind(testHost, "127.0.0.1")
	RotateTCPHostCache(testHost)
	InvalidateTCPHostCache(testHost)
	conn3, err := dialContext(ctx, "tcp4", net.JoinHostPort(testHost, portStr))
	if err != nil {
		t.Fatalf("dialContext with permanent bind failed: %v", err)
	}
	_ = conn3.Close()
	if lookupCalls.Load() != 2 {
		t.Fatalf("permanent bind should not re-query DNS, got %d lookups", lookupCalls.Load())
	}
}

func TestCheckProxyAddrAndProxyHostnameRules(t *testing.T) {
	validCases := []struct {
		in         string
		wantScheme string
		wantHost   string
	}{
		{"127.0.0.1:7897", "http", "127.0.0.1:7897"},
		{"[::1]:7897", "http", "[::1]:7897"},
		{"http://127.0.0.1:7897", "http", "127.0.0.1:7897"},
		{"http://myproxy.local", "http", "myproxy.local"},
		{"https://myproxy.local:8443", "https", "myproxy.local:8443"},
		{"socks5://127.0.0.1:1080", "socks5", "127.0.0.1:1080"},
		{"socks5h://127.0.0.1:1080", "socks5h", "127.0.0.1:1080"},
		{"socks5://myproxy.local", "socks5", "myproxy.local"},
		{"http://[::1]", "http", "[::1]"},
	}
	for _, tc := range validCases {
		u, err := checkProxyAddr(tc.in)
		if err != nil {
			t.Fatalf("checkProxyAddr(%q) unexpected error: %v", tc.in, err)
		}
		if u.Scheme != tc.wantScheme || u.Host != tc.wantHost {
			t.Fatalf("checkProxyAddr(%q) = (scheme=%q, host=%q), want (%q, %q)",
				tc.in, u.Scheme, u.Host, tc.wantScheme, tc.wantHost)
		}
	}

	for _, bad := range []string{
		"", "   ", "not_a_proxy", "127.0.0.1:0", "127.0.0.1:70000", "://missing-scheme",
		"http://127.0.0.1:0", "http://127.0.0.1:70000", "http://127.0.0.1:abc",
		"http://:8080", "http://myproxy.local:", "ftp://127.0.0.1:21",
	} {
		if u, err := checkProxyAddr(bad); err == nil {
			t.Fatalf("checkProxyAddr(%q) expected error, got %#v", bad, u)
		}
	}

	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("https_proxy", "")
	defer func() {
		SetGlobalProxy("")
		SetProxyHostnameRules("")
	}()

	SetGlobalProxy("127.0.0.1:7897")
	SetProxyHostnameRules("pan.baidu.com, *.pcs.baidu.com")

	checkProxy := func(rawURL string, wantProxied bool) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := proxyFunc(req)
		if err != nil {
			t.Fatalf("proxyFunc(%q) error: %v", rawURL, err)
		}
		if wantProxied && (got == nil || got.Host != "127.0.0.1:7897" || got.Scheme != "http") {
			t.Fatalf("proxyFunc(%q) = %v, want http://127.0.0.1:7897", rawURL, got)
		}
		if !wantProxied && got != nil {
			t.Fatalf("proxyFunc(%q) = %v, want nil (direct/env)", rawURL, got)
		}
	}

	checkProxy("https://pan.baidu.com/disk/main", true)
	checkProxy("https://d.pan.baidu.com/file/123", true)
	checkProxy("https://c3.pcs.baidu.com/rest/2.0/pcs/file", true)
	checkProxy("https://passport.baidu.com/v2/api/getqrcode", false)
	checkProxy("https://baidu.com/", false)
	checkProxy("https://notpan.baidu.com/", false)

	// Clearing a per-client SetProxy("") must restore proxyFunc (honoring global ProxyAddr/ProxyHostnameRules).
	hc := NewHTTPClient()
	hc.SetProxy("127.0.0.1:9999")
	hc.SetProxy("")
	panReq, _ := http.NewRequest(http.MethodGet, "https://pan.baidu.com/disk/main", nil)
	gotAfterClear, err := hc.currentTransport().Proxy(panReq)
	if err != nil || gotAfterClear == nil || gotAfterClear.Host != "127.0.0.1:7897" {
		t.Fatalf("SetProxy(\"\") wiped global proxyFunc: got (%v, %v), want 127.0.0.1:7897", gotAfterClear, err)
	}
}

func TestMixedTCPDeadAndTLSEOFRotationEvictsWithoutInfiniteLoop(t *testing.T) {
	// Reproduces [IP_A (accepts TCP SYN, drops TLS -> EOF), IP_B (TCP refused), IP_C (accepts TCP SYN, drops TLS -> EOF)]:
	// promoteIP must NOT reset rotated=0 during an active rotation cycle, or tcpCache will ping-pong forever and never re-query DNS.
	lnGood, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lnGood.Close()
	_, portStr, _ := net.SplitHostPort(lnGood.Addr().String())

	lnEOFA, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.2", portStr))
	if err != nil {
		t.Skipf("loopback 127.0.0.2 listen unavailable on this OS: %v", err)
	}
	defer lnEOFA.Close()

	lnEOFC, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.3", portStr))
	if err != nil {
		t.Skipf("loopback 127.0.0.3 listen unavailable on this OS: %v", err)
	}
	defer lnEOFC.Close()

	acceptAndClose := func(ln net.Listener, tag byte) {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			if tag != 0 {
				_, _ = c.Write([]byte{tag})
			}
			_ = c.Close()
		}
	}
	go acceptAndClose(lnEOFA, 0)
	go acceptAndClose(lnEOFC, 0)
	go acceptAndClose(lnGood, 'G')

	const testHost = "pingpong-evict-test.baidu.internal"
	var lookups atomic.Int32
	dialConfigMu.Lock()
	origLookup := dnsLookupHostFunc
	dnsLookupHostFunc = func(_ context.Context, host string) ([]string, error) {
		if strings.EqualFold(host, testHost) {
			if lookups.Add(1) == 1 {
				// 127.0.0.2 = TLS EOF, 127.0.0.4 = TCP connection refused, 127.0.0.3 = TLS EOF
				return []string{"127.0.0.2", "127.0.0.4", "127.0.0.3"}, nil
			}
			return []string{"127.0.0.1"}, nil
		}
		return origLookup(context.Background(), host)
	}
	dialConfigMu.Unlock()
	defer func() {
		dialConfigMu.Lock()
		dnsLookupHostFunc = origLookup
		dialConfigMu.Unlock()
		InvalidateTCPHostCache(testHost)
	}()
	InvalidateTCPHostCache(testHost)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var gotGood bool
	for attempt := 1; attempt <= 4; attempt++ {
		conn, err := dialContext(ctx, "tcp4", net.JoinHostPort(testHost, portStr))
		if err != nil {
			RotateTCPHostCache(testHost)
			continue
		}
		var b [1]byte
		n, readErr := conn.Read(b[:])
		_ = conn.Close()
		if n == 1 && b[0] == 'G' {
			gotGood = true
			break
		}
		if readErr == nil {
			t.Fatalf("unexpected non-EOF read on attempt %d", attempt)
		}
		RotateTCPHostCache(testHost)
	}
	if !gotGood {
		t.Fatalf("tcpCache got stuck in promoteIP/RotateTCPHostCache loop; lookups=%d", lookups.Load())
	}
	if lookups.Load() != 2 {
		t.Fatalf("expected exactly 2 DNS lookups (initial + post-eviction), got %d", lookups.Load())
	}
}

func TestLocalTCPAddrListWhitespaceTrimDualStackFilterAndConcurrentDNSAndHTTPSecure(t *testing.T) {
	defer SetLocalTCPAddrList()

	// 1. Whitespace-padded and bracketed local_addrs must parse cleanly, and IPv4-only local_addrs must NOT leak unbound over IPv6!
	SetLocalTCPAddrList(" 127.0.0.1 ", " ")
	if got := getLocalTCPAddrForIP(net.ParseIP("127.0.0.1")); got == nil || !got.IP.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("SetLocalTCPAddrList failed to trim whitespace around IPv4 addr: got %#v", got)
	}

	lnV4, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lnV4.Close()
	_, portV4, _ := net.SplitHostPort(lnV4.Addr().String())
	go func() {
		for {
			c, err := lnV4.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	var ipv6Hit atomic.Bool
	if lnV6, v6Err := net.Listen("tcp6", net.JoinHostPort("::1", portV4)); v6Err == nil {
		defer lnV6.Close()
		go func() {
			for {
				c, err := lnV6.Accept()
				if err != nil {
					return
				}
				ipv6Hit.Store(true)
				_ = c.Close()
			}
		}()
	}

	const dualHost = "dualstack-localaddr.baidu.internal"
	const v6OnlyHost = "v6only-localaddr.baidu.internal"
	const slowHost = "slow-dns.baidu.internal"
	const fastHost = "fast-dns.baidu.internal"

	dialConfigMu.Lock()
	origLookup := dnsLookupHostFunc
	dnsLookupHostFunc = func(ctx context.Context, host string) ([]string, error) {
		switch strings.ToLower(host) {
		case dualHost:
			return []string{"::1", "127.0.0.1"}, nil
		case v6OnlyHost:
			return []string{"::1"}, nil
		case slowHost:
			select {
			case <-time.After(400 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []string{"127.0.0.1"}, nil
		case fastHost:
			return []string{"127.0.0.1"}, nil
		default:
			return origLookup(ctx, host)
		}
	}
	dialConfigMu.Unlock()
	defer func() {
		dialConfigMu.Lock()
		dnsLookupHostFunc = origLookup
		dialConfigMu.Unlock()
		InvalidateTCPHostCache(dualHost)
		InvalidateTCPHostCache(v6OnlyHost)
		InvalidateTCPHostCache(slowHost)
		InvalidateTCPHostCache(fastHost)
	}()

	conn, err := dialContext(context.Background(), "tcp", net.JoinHostPort(dualHost, portV4))
	if err != nil {
		t.Fatalf("dialContext with IPv4 local_addrs and [::1, 127.0.0.1] failed: %v", err)
	}
	_ = conn.Close()
	if ipv6Hit.Load() {
		t.Fatal("dialContext leaked onto unbound IPv6 listener despite IPv4-only SetLocalTCPAddrList")
	}

	if badConn, v6OnlyErr := dialContext(context.Background(), "tcp", net.JoinHostPort(v6OnlyHost, portV4)); v6OnlyErr == nil {
		_ = badConn.Close()
		t.Fatal("expected IPv6-only host to fail when local_addrs is IPv4-only rather than bypassing local_addrs")
	}
	SetLocalTCPAddrList()

	// 2. Slow DNS lookup for slowHost must NOT block concurrent DNS lookup for fastHost.
	slowDone := make(chan struct{})
	go func() {
		defer close(slowDone)
		c, _ := dialContext(context.Background(), "tcp4", net.JoinHostPort(slowHost, portV4))
		if c != nil {
			_ = c.Close()
		}
	}()
	time.Sleep(25 * time.Millisecond)
	startFast := time.Now()
	fastConn, err := dialContext(context.Background(), "tcp4", net.JoinHostPort(fastHost, portV4))
	fastElapsed := time.Since(startFast)
	if err != nil {
		t.Fatalf("fastHost dialContext failed: %v", err)
	}
	_ = fastConn.Close()
	<-slowDone
	if fastElapsed > 200*time.Millisecond {
		t.Fatalf("fastHost DNS lookup was blocked behind slowHost lookup (%v)", fastElapsed)
	}

	// 3. Concurrent SetHTTPSecure + Get/Do calls must be data-race-free under -race.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	hc := NewHTTPClient()
	defer hc.CloseIdleConnections()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				hc.SetHTTPSecure(idx%4 == 0)
			}
			resp, getErr := hc.Get(srv.URL)
			if getErr != nil {
				t.Errorf("concurrent Get failed: %v", getErr)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}(i)
	}
	wg.Wait()
}
