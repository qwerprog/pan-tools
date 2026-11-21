package requester

import (
	"context"
	"errors"
	"fmt"
	mathrand "math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/dnscache"
	"golang.org/x/sync/singleflight"
)

const (
	// MaxDuration 最大的Duration
	MaxDuration = 1<<63 - 1
)

type tcpHostCacheEntry struct {
	ips       []net.IP
	permanent bool
	expiresAt time.Time
	rotated   int
}

type tcpHostCache struct {
	mu    sync.RWMutex
	items map[string]tcpHostCacheEntry
	group singleflight.Group
}

func newTCPHostCache() *tcpHostCache {
	return &tcpHostCache{
		items: make(map[string]tcpHostCacheEntry),
	}
}

func normalizeCacheHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

func cloneIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

func cloneIPSlice(ips []net.IP) []net.IP {
	if len(ips) == 0 {
		return nil
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip != nil {
			out = append(out, cloneIP(ip))
		}
	}
	return out
}

func (c *tcpHostCache) storePermanent(host string, ip net.IP) {
	key := normalizeCacheHost(host)
	c.mu.Lock()
	if ip == nil {
		delete(c.items, key)
	} else {
		c.items[key] = tcpHostCacheEntry{
			ips:       []net.IP{cloneIP(ip)},
			permanent: true,
		}
	}
	c.mu.Unlock()
}

func (c *tcpHostCache) getOrLoad(host string, ttl time.Duration, loader func() (net.IP, error)) (net.IP, error) {
	ips, err := c.getOrLoadIPs(context.Background(), host, ttl, func(context.Context) ([]net.IP, error) {
		ip, loadErr := loader()
		if loadErr != nil {
			return nil, loadErr
		}
		if ip == nil {
			return nil, fmt.Errorf("nil IP address for host %q", host)
		}
		return []net.IP{ip}, nil
	})
	if err != nil {
		return nil, err
	}
	return ips[0], nil
}

func (c *tcpHostCache) getOrLoadIPs(ctx context.Context, host string, ttl time.Duration, loader func(context.Context) ([]net.IP, error)) ([]net.IP, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	key := normalizeCacheHost(host)
	now := time.Now()
	c.mu.RLock()
	entry, ok := c.items[key]
	var cached []net.IP
	if ok && len(entry.ips) > 0 && (entry.permanent || now.Before(entry.expiresAt)) {
		cached = cloneIPSlice(entry.ips)
	}
	c.mu.RUnlock()
	if len(cached) > 0 {
		return cached, nil
	}

	ch := c.group.DoChan(key, func() (interface{}, error) {
		c.mu.RLock()
		cur, exists := c.items[key]
		var curIPs []net.IP
		if exists && len(cur.ips) > 0 && (cur.permanent || time.Now().Before(cur.expiresAt)) {
			curIPs = cloneIPSlice(cur.ips)
		}
		c.mu.RUnlock()
		if len(curIPs) > 0 {
			return curIPs, nil
		}

		lookupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		ips, loadErr := loader(lookupCtx)
		if loadErr != nil {
			return nil, loadErr
		}
		cleaned := cloneIPSlice(ips)
		if len(cleaned) == 0 {
			return nil, fmt.Errorf("no IP addresses found for host %q", host)
		}

		c.mu.Lock()
		// 仅当期间未被手动绑定永久地址时写入 TTL 缓存
		if existing, has := c.items[key]; !has || !existing.permanent {
			c.items[key] = tcpHostCacheEntry{
				ips:       cleaned,
				expiresAt: time.Now().Add(ttl),
			}
		} else {
			cleaned = cloneIPSlice(existing.ips)
		}
		c.mu.Unlock()
		return cleaned, nil
	})

	if ctx == nil {
		res := <-ch
		if res.Err != nil {
			return nil, res.Err
		}
		return cloneIPSlice(res.Val.([]net.IP)), nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return cloneIPSlice(res.Val.([]net.IP)), nil
	}
}

func (c *tcpHostCache) promoteIP(host string, goodIP net.IP, skippedFailed int) {
	if goodIP == nil {
		return
	}
	key := normalizeCacheHost(host)
	c.mu.Lock()
	defer c.mu.Unlock()
	cur, ok := c.items[key]
	if !ok || cur.permanent || len(cur.ips) <= 1 {
		return
	}
	idx := -1
	for i, existing := range cur.ips {
		if existing.Equal(goodIP) {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return
	}
	reordered := make([]net.IP, 0, len(cur.ips))
	reordered = append(reordered, cur.ips[idx:]...)
	reordered = append(reordered, cur.ips[:idx]...)
	cur.ips = reordered
	if cur.rotated > 0 && skippedFailed > 0 {
		cur.rotated += skippedFailed
	}
	c.items[key] = cur
}

func (c *tcpHostCache) rotateOrEvict(host string) {
	key := normalizeCacheHost(host)
	c.mu.Lock()
	defer c.mu.Unlock()
	cur, ok := c.items[key]
	if !ok || cur.permanent {
		return
	}
	if len(cur.ips) <= 1 || cur.rotated+1 >= len(cur.ips) {
		delete(c.items, key)
		return
	}
	rotated := make([]net.IP, 0, len(cur.ips))
	rotated = append(rotated, cur.ips[1:]...)
	rotated = append(rotated, cur.ips[0])
	cur.ips = rotated
	cur.rotated++
	c.items[key] = cur
}

func (c *tcpHostCache) invalidate(host string) {
	key := normalizeCacheHost(host)
	c.mu.Lock()
	if cur, ok := c.items[key]; ok && !cur.permanent {
		delete(c.items, key)
	}
	c.mu.Unlock()
}

var (
	dialConfigMu     sync.RWMutex
	localTCPAddrList = []*net.TCPAddr{}

	// ProxyAddr 代理地址
	ProxyAddr string

	// 指定走ProxyAddr的域名
	ProxyHostnameRules string

	// ErrProxyAddrEmpty 代理地址为空
	ErrProxyAddrEmpty = errors.New("proxy addr is empty")

	tcpCache = newTCPHostCache()

	// 保留 dnscache 类型引用以兼容 go.mod 依赖，实际查询统一走带 TTL 与多 IP 轮转的 tcpCache + 标准库 Resolver。
	_ = (*dnscache.Resolver)(nil)

	dnsLookupHostFunc = defaultDNSLookupHost
)

func defaultDNSLookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

// SetLocalTCPAddrList 设置网卡地址
func SetLocalTCPAddrList(ips ...string) {
	list := make([]*net.TCPAddr, 0, len(ips))
	for k := range ips {
		raw := strings.TrimSpace(ips[k])
		raw = strings.TrimPrefix(strings.TrimSuffix(raw, "]"), "[")
		if raw == "" {
			continue
		}
		p := net.ParseIP(raw)
		if p == nil {
			continue
		}

		list = append(list, &net.TCPAddr{
			IP: p,
		})
	}
	dialConfigMu.Lock()
	localTCPAddrList = list
	dialConfigMu.Unlock()
}

func matchProxyHostnameRules(rules, reqHostname string) bool {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(reqHostname), "."))
	if host == "" {
		return false
	}
	tokens := strings.FieldsFunc(rules, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})
	for _, tok := range tokens {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			continue
		}
		if tok == "*" {
			return true
		}
		tok = strings.TrimPrefix(tok, "*.")
		tok = strings.TrimPrefix(tok, ".")
		tok = strings.TrimSuffix(tok, ".")
		if tok == "" {
			continue
		}
		if host == tok || strings.HasSuffix(host, "."+tok) {
			return true
		}
	}
	return false
}

func proxyFunc(req *http.Request) (*url.URL, error) {
	dialConfigMu.RLock()
	proxyAddr := ProxyAddr
	rules := ProxyHostnameRules
	dialConfigMu.RUnlock()

	u, err := checkProxyAddr(proxyAddr)
	if err != nil {
		return http.ProxyFromEnvironment(req)
	}

	if strings.TrimSpace(rules) != "" {
		if req != nil && req.URL != nil && matchProxyHostnameRules(rules, req.URL.Hostname()) {
			return u, nil
		}
		return http.ProxyFromEnvironment(req)
	}
	return u, nil
}

func getLocalTCPAddrForIP(dstIP net.IP) *net.TCPAddr {
	dialConfigMu.RLock()
	defer dialConfigMu.RUnlock()
	if len(localTCPAddrList) == 0 {
		return nil
	}
	if dstIP == nil {
		return localTCPAddrList[mathrand.Intn(len(localTCPAddrList))]
	}
	wantV4 := dstIP.To4() != nil
	matched := make([]*net.TCPAddr, 0, len(localTCPAddrList))
	for _, addr := range localTCPAddrList {
		if addr != nil && addr.IP != nil && (addr.IP.To4() != nil) == wantV4 {
			matched = append(matched, addr)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	return matched[mathrand.Intn(len(matched))]
}

func getDialer() *net.Dialer {
	return getDialerForNetwork("tcp")
}

func getDialerForNetwork(network string) *net.Dialer {
	return getDialerForIP(network, nil)
}

func getDialerForIP(network string, dstIP net.IP) *net.Dialer {
	d := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	if strings.HasPrefix(network, "tcp") {
		if localAddr := getLocalTCPAddrForIP(dstIP); localAddr != nil {
			d.LocalAddr = localAddr
		}
	}
	return d
}

func isSupportedProxyScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "http", "https", "socks5", "socks5h":
		return true
	default:
		return false
	}
}

func checkProxyAddr(proxyAddr string) (u *url.URL, err error) {
	proxyAddr = strings.TrimSpace(proxyAddr)
	if proxyAddr == "" {
		return nil, ErrProxyAddrEmpty
	}

	if !strings.Contains(proxyAddr, "://") {
		host, port, splitErr := net.SplitHostPort(proxyAddr)
		if splitErr == nil {
			portNum, portErr := strconv.Atoi(port)
			if host != "" && portErr == nil && portNum > 0 && portNum <= 65535 {
				return &url.URL{
					Scheme: "http",
					Host:   net.JoinHostPort(host, port),
				}, nil
			}
		}
		return nil, fmt.Errorf("invalid proxy addr %q", proxyAddr)
	}

	u, err = url.Parse(proxyAddr)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" || u.Hostname() == "" || !isSupportedProxyScheme(u.Scheme) {
		return nil, fmt.Errorf("invalid proxy addr %q", proxyAddr)
	}
	if host, port, splitErr := net.SplitHostPort(u.Host); splitErr == nil {
		portNum, portErr := strconv.Atoi(port)
		if host == "" || portErr != nil || portNum <= 0 || portNum > 65535 {
			return nil, fmt.Errorf("invalid proxy addr %q", proxyAddr)
		}
	} else if u.Host != u.Hostname() && u.Host != "["+u.Hostname()+"]" {
		return nil, fmt.Errorf("invalid proxy addr %q", proxyAddr)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	return u, nil
}

// SetGlobalProxy 设置代理
func SetGlobalProxy(proxyAddr string) {
	dialConfigMu.Lock()
	ProxyAddr = proxyAddr
	dialConfigMu.Unlock()
}

// SetProxyHostnameRules 设置走代理的域名列表
func SetProxyHostnameRules(hostnames string) {
	dialConfigMu.Lock()
	ProxyHostnameRules = hostnames
	dialConfigMu.Unlock()
}

// SetTCPHostBind 设置host绑定ip
func SetTCPHostBind(host, ip string) {
	tcpCache.storePermanent(host, net.ParseIP(strings.TrimSpace(ip)))
}

// RotateTCPHostCache 在连接发生 TLS/HTTP 瞬态中断（如 EOF、重置或握手超时）时轮转候选 IP；
// 当所有已缓存候选 IP 均轮转完毕时自动淘汰缓存，以便下次请求重新触发 DNS 解析。
func RotateTCPHostCache(host string) {
	if host == "" {
		return
	}
	tcpCache.rotateOrEvict(host)
}

// InvalidateTCPHostCache 清除指定非永久绑定域名的缓存 IP。
func InvalidateTCPHostCache(host string) {
	if host == "" {
		return
	}
	tcpCache.invalidate(host)
}

func interleaveResolvedIPs(addrs []string) []net.IP {
	seen := make(map[string]struct{}, len(addrs))
	var v4, v6 []net.IP
	preferV6First := false
	firstSeen := false
	for _, raw := range addrs {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil {
			continue
		}
		canon := ip.String()
		if _, dup := seen[canon]; dup {
			continue
		}
		seen[canon] = struct{}{}
		isV4 := ip.To4() != nil
		if !firstSeen {
			firstSeen = true
			preferV6First = !isV4
		}
		if isV4 {
			v4 = append(v4, ip)
		} else {
			v6 = append(v6, ip)
		}
	}
	primary, secondary := v4, v6
	if preferV6First {
		primary, secondary = v6, v4
	}
	out := make([]net.IP, 0, len(primary)+len(secondary))
	for i := 0; i < len(primary) || i < len(secondary); i++ {
		if i < len(primary) {
			out = append(out, primary[i])
		}
		if i < len(secondary) {
			out = append(out, secondary[i])
		}
	}
	return out
}

func resolveTCPHostIPs(ctx context.Context, host string) (ips []net.IP, err error) {
	// host 已是字面量 IP 时直接返回，无需 DNS 查询
	if parsed := net.ParseIP(host); parsed != nil {
		return []net.IP{parsed}, nil
	}

	// 标准库 DNS 偶发 panic 时转为 error，避免进程崩溃
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("dns lookup panic for %q: %v", host, r)
		}
	}()

	dialConfigMu.RLock()
	lookupFn := dnsLookupHostFunc
	dialConfigMu.RUnlock()

	addrs, lookupErr := lookupFn(ctx, host)
	if lookupErr != nil {
		return nil, lookupErr
	}
	ips = interleaveResolvedIPs(addrs)
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses found for host %q", host)
	}
	return ips, nil
}

func filterIPsForNetwork(network string, ips []net.IP) []net.IP {
	switch network {
	case "tcp4":
		out := make([]net.IP, 0, len(ips))
		for _, ip := range ips {
			if ip != nil && ip.To4() != nil {
				out = append(out, ip)
			}
		}
		return out
	case "tcp6":
		out := make([]net.IP, 0, len(ips))
		for _, ip := range ips {
			if ip != nil && ip.To4() == nil && ip.To16() != nil {
				out = append(out, ip)
			}
		}
		return out
	default:
		return ips
	}
}

func filterIPsForLocalAddrs(ips []net.IP) ([]net.IP, bool) {
	dialConfigMu.RLock()
	if len(localTCPAddrList) == 0 {
		dialConfigMu.RUnlock()
		return ips, false
	}
	var hasV4, hasV6 bool
	for _, addr := range localTCPAddrList {
		if addr == nil || addr.IP == nil {
			continue
		}
		if addr.IP.To4() != nil {
			hasV4 = true
		} else if addr.IP.To16() != nil {
			hasV6 = true
		}
	}
	dialConfigMu.RUnlock()

	if !hasV4 && !hasV6 {
		return ips, false
	}
	if hasV4 && hasV6 {
		return ips, true
	}
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		isV4 := ip.To4() != nil
		if (isV4 && hasV4) || (!isV4 && hasV6) {
			out = append(out, ip)
		}
	}
	return out, true
}

func boundedCandidateDialContext(ctx context.Context, remaining int) (context.Context, context.CancelFunc) {
	if remaining <= 1 {
		return ctx, func() {}
	}
	const maxPerCandidate = 8 * time.Second
	perAttempt := maxPerCandidate
	if deadline, ok := ctx.Deadline(); ok {
		rem := time.Until(deadline)
		if rem <= 0 {
			return ctx, func() {}
		}
		if share := rem / time.Duration(remaining); share < perAttempt {
			perAttempt = share
		}
	}
	return context.WithTimeout(ctx, perAttempt)
}

func dialContext(ctx context.Context, network, address string) (conn net.Conn, err error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	switch network {
	case "tcp", "tcp4", "tcp6":
		host, portStr, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, err
		}

		ips, err := tcpCache.getOrLoadIPs(ctx, host, 10*time.Minute, func(lookupCtx context.Context) ([]net.IP, error) {
			return resolveTCPHostIPs(lookupCtx, host)
		})
		if err != nil {
			return nil, err
		}

		candidates := filterIPsForNetwork(network, ips)
		if len(candidates) == 0 {
			return nil, fmt.Errorf("no %s IP addresses found for host %q", network, host)
		}
		candidates, hasLocalBind := filterIPsForLocalAddrs(candidates)
		if hasLocalBind && len(candidates) == 0 {
			return nil, fmt.Errorf("no IP addresses matching configured local_addrs family for host %q", host)
		}

		portFormatted := strconv.Itoa(port)
		var lastErr error
		for i, ip := range candidates {
			if ctx != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return nil, ctxErr
				}
			}
			attemptCtx, cancelAttempt := boundedCandidateDialContext(ctx, len(candidates)-i)
			target := net.JoinHostPort(ip.String(), portFormatted)
			conn, lastErr = getDialerForIP(network, ip).DialContext(attemptCtx, network, target)
			cancelAttempt()
			if lastErr == nil {
				if i > 0 {
					tcpCache.promoteIP(host, ip, i)
				}
				return conn, nil
			}
		}
		return nil, lastErr
	}

	// 非 tcp 请求
	conn, err = getDialerForNetwork(network).DialContext(ctx, network, address)
	return
}
