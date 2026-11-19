package pcscommand

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	jsoniter "github.com/json-iterator/go"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
	"github.com/tidwall/gjson"
	"golang.org/x/term"
)

const (
	qrDesktopUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	defaultPassportBase  = "https://passport.baidu.com"
	defaultPanBase       = "https://pan.baidu.com"
	defaultQRTotalWait   = 180 * time.Second
	defaultQRPollTimeout = 40 * time.Second
	defaultQRPollSleep   = 150 * time.Millisecond
	qrCodeFileName       = "qrcode.png"
)

var (
	// ErrLoginCanceled 表示用户主动按 Ctrl+C 等中断了交互式或扫码登录流程。
	ErrLoginCanceled = errors.New("已取消登录")

	netdiskStokenRe = regexp.MustCompile(`netdisk#([0-9a-zA-Z\-_*]+)`)
	quotedBDUSSRe   = regexp.MustCompile(`['"]bduss['"]\s*:\s*['"]([^'"]+)['"]`)
	quotedPTokenRe  = regexp.MustCompile(`['"]ptoken['"]\s*:\s*['"]([^'"]+)['"]`)

	qrDefaultOpts = qrLoginOptions{
		passportBaseURL: defaultPassportBase,
		panBaseURL:      defaultPanBase,
		totalTimeout:    defaultQRTotalWait,
		pollTimeout:     defaultQRPollTimeout,
		pollInterval:    defaultQRPollSleep,
	}
)

type qrLoginOptions struct {
	passportBaseURL string
	panBaseURL      string
	totalTimeout    time.Duration
	pollTimeout     time.Duration
	pollInterval    time.Duration
	errOut          io.Writer
	onQRReady       func(qrPath string)
}

// OverrideQRLoginEndpointsForTest 供跨包端到端测试注入本地 httptest 服务地址与轮询超时，返回恢复闭包。
func OverrideQRLoginEndpointsForTest(passportBase, panBase string, pollInterval, totalTimeout time.Duration) func() {
	prev := qrDefaultOpts
	if passportBase != "" {
		qrDefaultOpts.passportBaseURL = passportBase
	}
	if panBase != "" {
		qrDefaultOpts.panBaseURL = panBase
	}
	if pollInterval > 0 {
		qrDefaultOpts.pollInterval = pollInterval
	}
	if totalTimeout > 0 {
		qrDefaultOpts.totalTimeout = totalTimeout
	}
	return func() {
		qrDefaultOpts = prev
	}
}

// RunQRLogin 执行百度 Passport + 百度网盘 (tpl=netdisk) 扫码登录完整流程，
// 自动获取 BDUSS 与网盘专属 STOKEN 并去重组装完整 Cookies。
func RunQRLogin() (bduss, ptoken, stoken, cookies string, err error) {
	opts := qrDefaultOpts
	opts.errOut = os.Stderr
	return runQRLoginWithOptions(context.Background(), opts)
}

func runQRLoginWithOptions(parentCtx context.Context, opts qrLoginOptions) (bduss, ptoken, stoken, cookies string, err error) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	passportBase := strings.TrimRight(strings.TrimSpace(opts.passportBaseURL), "/")
	if passportBase == "" {
		passportBase = defaultPassportBase
	}
	panBase := strings.TrimRight(strings.TrimSpace(opts.panBaseURL), "/")
	if panBase == "" {
		panBase = defaultPanBase
	}
	totalTimeout := opts.totalTimeout
	if totalTimeout <= 0 {
		totalTimeout = defaultQRTotalWait
	}
	pollTimeout := opts.pollTimeout
	if pollTimeout <= 0 {
		pollTimeout = defaultQRPollTimeout
	}
	pollInterval := opts.pollInterval
	if pollInterval <= 0 {
		pollInterval = defaultQRPollSleep
	}
	errOut := opts.errOut
	if errOut == nil {
		errOut = os.Stderr
	}

	passportParsed, err := url.Parse(passportBase)
	if err != nil {
		return "", "", "", "", fmt.Errorf("解析 Passport 地址失败: %w", err)
	}
	panParsed, err := url.Parse(panBase)
	if err != nil {
		return "", "", "", "", fmt.Errorf("解析网盘地址失败: %w", err)
	}

	sigCtx, stopSig := signal.NotifyContext(parentCtx, os.Interrupt)
	defer stopSig()

	totalCtx, cancelTotal := context.WithTimeout(sigCtx, totalTimeout)
	defer cancelTotal()

	// Step 0: 创建独立 HTTPS 安全客户端并预热 pan.baidu.com 以获取初始 BAIDUID 会话 Cookie
	client := requester.NewHTTPClient()
	client.SetHTTPSecure(true)
	client.SetTLSHandshakeTimeout(5 * time.Second)
	client.SetResponseHeaderTimeout(45 * time.Second)
	client.SetTimeout(60 * time.Second)
	client.SetUserAgent(qrDesktopUserAgent)
	jar, _ := cookiejar.New(nil)
	client.SetCookiejar(jar)

	prewarmPanSession(sigCtx, totalCtx, client, panBase)
	if sigCtx.Err() != nil {
		return "", "", "", "", ErrLoginCanceled
	}

	// Step 1: 请求 getqrcode 并拉取二维码 PNG 图片
	gid := generateQRGID()
	qrMetaURL := fmt.Sprintf("%s/v2/api/getqrcode?lp=pc&qrloginfrom=pc&gid=%s&tpl=netdisk&apiver=v3&tt=%d",
		passportBase, url.QueryEscape(gid), time.Now().UnixMilli())
	metaBody, err := fetchBytesWithRetry(sigCtx, totalCtx, client, qrMetaURL, 6*time.Second, 4, map[string]string{
		"Referer": panBase + "/",
		"Accept":  "application/json, text/javascript, */*; q=0.01",
	})
	if err != nil {
		return "", "", "", "", normalizeQRStepError(sigCtx, totalCtx, "获取百度扫码通道失败", err)
	}

	var qrMeta struct {
		Errno  int    `json:"errno"`
		Sign   string `json:"sign"`
		ImgURL string `json:"imgurl"`
	}
	if err := jsoniter.Unmarshal(unwrapJSONP(metaBody), &qrMeta); err != nil {
		return "", "", "", "", fmt.Errorf("解析二维码通道响应失败: %w", err)
	}
	if qrMeta.Errno != 0 || strings.TrimSpace(qrMeta.Sign) == "" || strings.TrimSpace(qrMeta.ImgURL) == "" {
		return "", "", "", "", fmt.Errorf("获取登录二维码失败 (errno=%d)", qrMeta.Errno)
	}

	imgURL := normalizeQRImgURL(qrMeta.ImgURL, passportBase)
	pngBytes, err := fetchBytesWithRetry(sigCtx, totalCtx, client, imgURL, 8*time.Second, 4, map[string]string{
		"Referer": panBase + "/",
	})
	if err != nil {
		return "", "", "", "", normalizeQRStepError(sigCtx, totalCtx, "下载登录二维码图片失败", err)
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return "", "", "", "", fmt.Errorf("解析登录二维码 PNG 失败: %w", err)
	}

	qrPath := filepath.Join(pcsconfig.GetConfigDir(), qrCodeFileName)
	if err := writePrivateQRFile(qrPath, pngBytes); err != nil {
		return "", "", "", "", fmt.Errorf("保存临时二维码文件失败: %w", err)
	}
	defer func() {
		_ = os.Remove(qrPath)
	}()

	asciiOnly := ui.ASCIIOnly()
	colorOn := ui.ColorEnabled(errOut)
	if asciiOnly {
		ui.PrintInfo(errOut, "当前处于纯 ASCII 模式，请使用百度网盘/百度 App 扫描本地二维码图片完成登录: %s", qrPath)
	} else {
		ui.PrintInfo(errOut, "请使用「百度网盘 App」或「百度 App」扫描下方二维码完成登录：")
		matrix, matErr := DecodeQRImageMatrix(img)
		if matErr != nil {
			ui.PrintWarn(errOut, "终端二维码矩阵提取失败 (%v)，请打开本地图片扫码: %s", matErr, qrPath)
		} else {
			rendered := RenderQRMatrixDense1x2(errOut, matrix, colorOn, false)
			if rendered != "" {
				fmt.Fprintln(errOut, rendered)
			} else if termCols := writerTTYCols(errOut); termCols > 0 {
				ui.PrintWarn(errOut, "当前终端宽度 (%d 列) 小于二维码宽度 (%d 列)，已跳过终端字符二维码以避免折行错位，请拉宽终端或打开本地图片扫码: %s",
					termCols, len(matrix[0])+2, qrPath)
			}
		}
		ui.PrintSuccess(errOut, "二维码图片已保存至 %s (登录结束或取消后自动清理)", qrPath)
	}
	ui.PrintInfo(errOut, "等待手机扫码确认 (按 Ctrl+C 取消)...")

	if opts.onQRReady != nil {
		opts.onQRReady(qrPath)
	}

	// Step 2: channel/unicast 三态长轮询
	ticketV, err := pollQRUnicast(sigCtx, totalCtx, client, passportBase, panBase, qrMeta.Sign, gid, pollTimeout, pollInterval, errOut)
	if err != nil {
		return "", "", "", "", err
	}

	// Step 3: qrbdusslogin 交换 BDUSS 及 stokenList 备选 netdisk#<stoken>
	panDiskMain := panBase + "/disk/main"
	qrLoginParams := url.Values{}
	qrLoginParams.Set("v", strconv.FormatInt(time.Now().UnixMilli(), 10))
	qrLoginParams.Set("bduss", ticketV)
	qrLoginParams.Set("u", panDiskMain)
	qrLoginParams.Set("loginVersion", "v4")
	qrLoginParams.Set("qrcode", "1")
	qrLoginParams.Set("tpl", "netdisk")
	qrLoginParams.Set("apiver", "v3")
	qrLoginParams.Set("tt", strconv.FormatInt(time.Now().UnixMilli(), 10))
	qrLoginParams.Set("traceid", "")
	qrLoginParams.Set("callback", "bd__cbs__qrlogin")

	qrLoginURL := passportBase + "/v3/login/main/qrbdusslogin?" + qrLoginParams.Encode()
	loginBody, err := fetchBytesWithRetry(sigCtx, totalCtx, client, qrLoginURL, 12*time.Second, 3, map[string]string{
		"Referer": panBase + "/",
	})
	if err != nil {
		return "", "", "", "", normalizeQRStepError(sigCtx, totalCtx, "交换扫码登录凭据失败", err)
	}

	var passportStoken string
	for _, ck := range mergeURLCookies(client.Jar, passportParsed, panParsed) {
		switch ck.Name {
		case "BDUSS":
			if ck.Value != "" {
				bduss = ck.Value
			}
		case "PTOKEN":
			if ck.Value != "" {
				ptoken = ck.Value
			}
		case "STOKEN":
			if ck.Value != "" {
				passportStoken = ck.Value
			}
		}
	}

	unwrappedLogin := unwrapJSONP(loginBody)
	stokenFromList := extractNetdiskStokenFromBody(loginBody)
	if bduss == "" {
		if v := strings.TrimSpace(gjson.GetBytes(unwrappedLogin, "data.bduss").String()); v != "" {
			bduss = v
		} else if m := quotedBDUSSRe.FindSubmatch(unwrappedLogin); len(m) >= 2 {
			bduss = string(m[1])
		}
	}
	if ptoken == "" {
		if v := strings.TrimSpace(gjson.GetBytes(unwrappedLogin, "data.ptoken").String()); v != "" {
			ptoken = v
		} else if m := quotedPTokenRe.FindSubmatch(unwrappedLogin); len(m) >= 2 {
			ptoken = string(m[1])
		}
	}
	if bduss == "" {
		errMsg := strings.TrimSpace(gjson.GetBytes(unwrappedLogin, "errInfo.msg").String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(gjson.GetBytes(unwrappedLogin, "message").String())
		}
		if errMsg != "" {
			return "", "", "", "", fmt.Errorf("扫码登录未返回有效 BDUSS: %s", errMsg)
		}
		return "", "", "", "", errors.New("扫码登录未返回有效 BDUSS")
	}

	// 确保 jar 中存在 BDUSS，以便 Step 4 跨域 SSO 携带
	seedHostCookieIfMissing(client.Jar, passportParsed, "BDUSS", bduss)
	seedHostCookieIfMissing(client.Jar, panParsed, "BDUSS", bduss)

	// Step 4: v3/login/api/auth 跟随 302 重定向到 pan.baidu.com 获取网盘专属 STOKEN
	authParams := url.Values{}
	authParams.Set("return_type", "3")
	authParams.Set("tpl", "netdisk")
	authParams.Set("u", panDiskMain)
	authURL := passportBase + "/v3/login/api/auth?" + authParams.Encode()

	panStokenFromAuth, step4OtherStoken, authErr := performNetdiskAuthSSO(sigCtx, totalCtx, client, authURL, panBase, panParsed, passportStoken)
	if authErr != nil {
		if sigCtx.Err() != nil {
			return "", "", "", "", ErrLoginCanceled
		}
		pcsverbose.Verbosef("QR Netdisk SSO redirect warning: %v\n", authErr)
	}

	netdiskStoken := panStokenFromAuth
	if netdiskStoken == "" {
		netdiskStoken = stokenFromList
	}
	if netdiskStoken == "" {
		netdiskStoken = step4OtherStoken
	}
	if netdiskStoken == "" {
		ui.PrintWarn(errOut, "未能获取到百度网盘专属 STOKEN，分享转存等部分功能可能受限")
	}

	cookies = buildDeduplicatedCookieString(client.Jar, passportParsed, panParsed, bduss, ptoken, netdiskStoken)
	return bduss, ptoken, netdiskStoken, cookies, nil
}

func pollQRUnicast(
	sigCtx, totalCtx context.Context,
	client *requester.HTTPClient,
	passportBase, panBase, sign, gid string,
	pollTimeout, pollInterval time.Duration,
	errOut io.Writer,
) (string, error) {
	scannedPrinted := false
	consecutiveNetErrs := 0

	for {
		if sigCtx.Err() != nil {
			return "", ErrLoginCanceled
		}
		if totalCtx.Err() != nil {
			return "", errors.New("扫码等待超时，请重新运行 login -qr")
		}

		params := url.Values{}
		params.Set("channel_id", sign)
		params.Set("tpl", "netdisk")
		params.Set("gid", gid)
		params.Set("apiver", "v3")
		params.Set("tt", strconv.FormatInt(time.Now().UnixMilli(), 10))
		params.Set("callback", "")
		pollURL := passportBase + "/channel/unicast?" + params.Encode()

		pollCtx, cancelPoll := context.WithTimeout(totalCtx, pollTimeout)
		body, err := fetchBytesWithContext(pollCtx, client, pollURL, 0, map[string]string{
			"Referer": panBase + "/",
		})
		pollTimedOut := pollCtx.Err() == context.DeadlineExceeded
		cancelPoll()
		if err != nil {
			if sigCtx.Err() != nil {
				return "", ErrLoginCanceled
			}
			if totalCtx.Err() != nil {
				return "", errors.New("扫码等待超时，请重新运行 login -qr")
			}
			if isTLSCertVerificationError(err) {
				return "", fmt.Errorf("扫码状态轮询 TLS 证书校验失败: %w", err)
			}
			if pollTimedOut {
				consecutiveNetErrs = 0
				continue
			}
			consecutiveNetErrs++
			resetClientHostState(client, pollURL)
			if consecutiveNetErrs >= 5 {
				return "", fmt.Errorf("扫码状态轮询网络异常: %w", err)
			}
			if !sleepWithContext(totalCtx, pollInterval) {
				if sigCtx.Err() != nil {
					return "", ErrLoginCanceled
				}
				return "", errors.New("扫码等待超时，请重新运行 login -qr")
			}
			continue
		}
		consecutiveNetErrs = 0

		var outer struct {
			Errno    int             `json:"errno"`
			ChannelV json.RawMessage `json:"channel_v"`
		}
		if err := jsoniter.Unmarshal(unwrapJSONP(body), &outer); err != nil {
			return "", fmt.Errorf("解析扫码轮询响应失败: %w", err)
		}

		switch outer.Errno {
		case 1:
			// 单次长轮询未产生新事件，继续轮询
			if !sleepWithContext(totalCtx, pollInterval) {
				if sigCtx.Err() != nil {
					return "", ErrLoginCanceled
				}
				return "", errors.New("扫码等待超时，请重新运行 login -qr")
			}
		case 0:
			inner, err := parseUnicastChannelV(outer.ChannelV)
			if err != nil {
				return "", fmt.Errorf("解析扫码通道状态失败: %w", err)
			}
			switch inner.Status {
			case 1:
				if !scannedPrinted {
					ui.PrintInfo(errOut, "已扫码，请在手机上确认登录...")
					scannedPrinted = true
				}
				if !sleepWithContext(totalCtx, pollInterval) {
					if sigCtx.Err() != nil {
						return "", ErrLoginCanceled
					}
					return "", errors.New("扫码等待超时，请重新运行 login -qr")
				}
			case 0:
				ticket := strings.TrimSpace(inner.V)
				if ticket == "" {
					return "", errors.New("扫码已确认但未返回有效临时票据")
				}
				return ticket, nil
			case 2:
				return "", errors.New("手机端已取消登录或二维码已过期，请重新扫码")
			default:
				return "", fmt.Errorf("二维码状态已失效 (status=%d)，请重新扫码", inner.Status)
			}
		default:
			return "", fmt.Errorf("扫码通道已过期或异常 (errno=%d)，请重新运行 login -qr", outer.Errno)
		}
	}
}

type unicastChannelState struct {
	Status int    `json:"status"`
	V      string `json:"v"`
}

func parseUnicastChannelV(raw json.RawMessage) (unicastChannelState, error) {
	var state unicastChannelState
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return state, errors.New("channel_v 为空")
	}
	if trimmed[0] == '"' {
		var innerStr string
		if err := json.Unmarshal(trimmed, &innerStr); err != nil {
			return state, err
		}
		if err := jsoniter.UnmarshalFromString(innerStr, &state); err != nil {
			return state, err
		}
		return state, nil
	}
	if err := jsoniter.Unmarshal(trimmed, &state); err != nil {
		return state, err
	}
	return state, nil
}

func performNetdiskAuthSSO(
	sigCtx, parentCtx context.Context,
	baseClient *requester.HTTPClient,
	authURL, panBase string,
	panParsed *url.URL,
	passportStoken string,
) (panStoken, otherStoken string, err error) {
	ssoClient := baseClient.Clone()
	inspectResp := func(resp *http.Response) {
		if resp == nil {
			return
		}
		var reqURL *url.URL
		if resp.Request != nil {
			reqURL = resp.Request.URL
		}
		onPan := isPanTargetURL(reqURL, panParsed)
		for _, ck := range resp.Cookies() {
			if ck.Name != "STOKEN" || isDeletedOrExpiredCookie(ck) {
				continue
			}
			ckDomain := strings.ToLower(strings.TrimPrefix(ck.Domain, "."))
			isPanDomain := ckDomain == "pan.baidu.com" || strings.HasSuffix(ckDomain, ".pan.baidu.com")
			if isPanDomain || (onPan && (ckDomain == "" || ck.Value != passportStoken)) {
				panStoken = ck.Value
			} else if ck.Value != passportStoken {
				otherStoken = ck.Value
			}
		}
	}

	ssoClient.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("SSO 重定向次数过多")
		}
		inspectResp(req.Response)
		return nil
	}

	var reqErr error
	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(parentCtx, 12*time.Second)
		resp, doErr := doContextGet(ctx, ssoClient, authURL, map[string]string{
			"Referer": panBase + "/",
		})
		if resp != nil {
			inspectResp(resp)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
			_ = resp.Body.Close()
		}
		cancel()
		reqErr = doErr
		if reqErr == nil || panStoken != "" || attempt >= 2 || !isRetryableQRTransportError(sigCtx, parentCtx, reqErr) {
			break
		}
		resetClientHostState(ssoClient, authURL)
		if panParsed != nil && panParsed.Hostname() != "" {
			requester.RotateTCPHostCache(panParsed.Hostname())
		}
		if !sleepWithContext(parentCtx, 150*time.Millisecond) {
			break
		}
	}
	return panStoken, otherStoken, reqErr
}

func isPanTargetURL(u, panParsed *url.URL) bool {
	if u == nil {
		return false
	}
	if panParsed != nil && strings.EqualFold(u.Host, panParsed.Host) {
		// 若单实例测试把 passport 与 pan 挂载在同一 host:port，区分非 /v3/login/api/auth 的目标路径
		if strings.HasPrefix(u.Path, "/v3/login/api/auth") {
			return false
		}
		return true
	}
	host := strings.ToLower(u.Hostname())
	return host == "pan.baidu.com" || strings.HasSuffix(host, ".pan.baidu.com")
}

func buildDeduplicatedCookieString(
	jar http.CookieJar,
	passportURL, panURL *url.URL,
	bduss, ptoken, netdiskStoken string,
) string {
	var order []string
	values := make(map[string]string)
	setKV := func(name, val string) {
		name = strings.TrimSpace(name)
		val = strings.Trim(strings.TrimSpace(val), `"`)
		if name == "" || val == "" {
			return
		}
		if _, exists := values[name]; !exists {
			order = append(order, name)
		}
		values[name] = val
	}

	for _, ck := range mergeURLCookies(jar, passportURL, panURL) {
		// 跳过 jar 中的 STOKEN（可能混有 Passport 作用域的 .baidu.com STOKEN），稍后统一写入经过校验的网盘 STOKEN
		if ck.Name == "STOKEN" {
			continue
		}
		setKV(ck.Name, ck.Value)
	}
	setKV("BDUSS", bduss)
	if ptoken != "" {
		setKV("PTOKEN", ptoken)
	}
	if netdiskStoken != "" {
		setKV("STOKEN", netdiskStoken)
	}

	parts := make([]string, 0, len(order))
	for _, k := range order {
		if v := values[k]; v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, "; ")
}

func mergeURLCookies(jar http.CookieJar, urls ...*url.URL) []*http.Cookie {
	if jar == nil {
		return nil
	}
	var out []*http.Cookie
	for _, u := range urls {
		if u == nil {
			continue
		}
		out = append(out, jar.Cookies(u)...)
	}
	return out
}

func seedHostCookieIfMissing(jar http.CookieJar, u *url.URL, name, value string) {
	if jar == nil || u == nil || name == "" || value == "" {
		return
	}
	for _, ck := range jar.Cookies(u) {
		if ck.Name == name && ck.Value != "" {
			return
		}
	}
	domain := ""
	if host := strings.ToLower(u.Hostname()); host == "baidu.com" || strings.HasSuffix(host, ".baidu.com") {
		domain = ".baidu.com"
	}
	jar.SetCookies(u, []*http.Cookie{{
		Name:   name,
		Value:  value,
		Path:   "/",
		Domain: domain,
	}})
}

type qrHTTPStatusError struct {
	StatusCode int
	Status     string
}

func (e *qrHTTPStatusError) Error() string {
	return fmt.Sprintf("HTTP %d %s", e.StatusCode, e.Status)
}

func prewarmPanSession(sigCtx, totalCtx context.Context, client *requester.HTTPClient, panBase string) {
	prewarmTotalCtx, cancelTotal := context.WithTimeout(totalCtx, 6*time.Second)
	defer cancelTotal()

	targetURL := panBase + "/"
	headers := map[string]string{
		"Referer": targetURL,
	}
	for attempt := 1; attempt <= 2; attempt++ {
		attemptCtx, cancelAttempt := context.WithTimeout(prewarmTotalCtx, 3*time.Second)
		preResp, preErr := doContextGet(attemptCtx, client, targetURL, headers)
		if preErr == nil && preResp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(preResp.Body, 64*1024))
			_ = preResp.Body.Close()
			cancelAttempt()
			return
		}
		if preResp != nil {
			_ = preResp.Body.Close()
		}
		cancelAttempt()
		if attempt >= 2 || !isRetryableQRTransportError(sigCtx, prewarmTotalCtx, preErr) {
			return
		}
		resetClientHostState(client, targetURL)
	}
}

func doContextGet(ctx context.Context, client *requester.HTTPClient, urlStr string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	if client.UserAgent != "" {
		req.Header.Set("User-Agent", client.UserAgent)
	}
	for k, v := range headers {
		if strings.EqualFold(k, "Host") {
			req.Host = v
		}
		req.Header.Set(k, v)
	}
	return client.Do(req)
}

func fetchBytesWithContext(parentCtx context.Context, client *requester.HTTPClient, urlStr string, timeout time.Duration, headers map[string]string) ([]byte, error) {
	ctx := parentCtx
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(parentCtx, timeout)
		defer cancel()
	}
	resp, err := doContextGet(ctx, client, urlStr, headers)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &qrHTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func fetchBytesWithRetry(
	sigCtx, totalCtx context.Context,
	client *requester.HTTPClient,
	urlStr string,
	perAttemptTimeout time.Duration,
	maxAttempts int,
	headers map[string]string,
) ([]byte, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	backoff := 120 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		body, err := fetchBytesWithContext(totalCtx, client, urlStr, perAttemptTimeout, headers)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if attempt >= maxAttempts || !isRetryableQRTransportError(sigCtx, totalCtx, err) {
			break
		}
		resetClientHostState(client, urlStr)
		pcsverbose.Verbosef("QR HTTP transient error on %s (attempt %d/%d): %v, retrying...\n", urlStr, attempt, maxAttempts, err)
		if !sleepWithContext(totalCtx, backoff) {
			break
		}
		if backoff < time.Second {
			backoff *= 2
		}
	}
	return nil, lastErr
}

func resetClientHostState(client *requester.HTTPClient, rawURL string) {
	if client != nil {
		client.CloseIdleConnections()
	}
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Hostname() != "" {
		requester.RotateTCPHostCache(parsed.Hostname())
	}
}

func isTLSCertVerificationError(err error) bool {
	if err == nil {
		return false
	}
	var certVerifyErr *tls.CertificateVerificationError
	if errors.As(err, &certVerifyErr) {
		return true
	}
	var unknownAuthErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthErr) {
		return true
	}
	var certInvalidErr x509.CertificateInvalidError
	if errors.As(err, &certInvalidErr) {
		return true
	}
	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return true
	}
	var sysRootsErr x509.SystemRootsError
	return errors.As(err, &sysRootsErr)
}

func isRetryableQRTransportError(sigCtx, totalCtx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if sigCtx != nil && sigCtx.Err() != nil {
		return false
	}
	if totalCtx != nil && totalCtx.Err() != nil {
		return false
	}
	var httpStatusErr *qrHTTPStatusError
	if errors.As(err, &httpStatusErr) {
		switch httpStatusErr.StatusCode {
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	if isTLSCertVerificationError(err) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if isTimeoutError(err) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Err != nil {
			msg := strings.ToLower(urlErr.Err.Error())
			if strings.Contains(msg, "unsupported protocol scheme") || strings.Contains(msg, "invalid control character") {
				return false
			}
		}
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.HasSuffix(msg, ": eof") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "goaway") ||
		strings.Contains(msg, "server closed idle connection")
}

func normalizeQRStepError(sigCtx, totalCtx context.Context, prefix string, err error) error {
	if sigCtx != nil && sigCtx.Err() != nil {
		return ErrLoginCanceled
	}
	if totalCtx != nil && totalCtx.Err() != nil {
		return errors.New("扫码等待超时，请重新运行 login -qr")
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func normalizeQRImgURL(raw, passportBase string) string {
	s := strings.TrimSpace(raw)
	scheme := "https"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(passportBase)), "http://") {
		scheme = "http"
	}
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "https://"):
		return s
	case strings.HasPrefix(lower, "http://"):
		if scheme == "https" {
			if parsed, err := url.Parse(s); err == nil {
				host := strings.ToLower(parsed.Hostname())
				if host == "baidu.com" || strings.HasSuffix(host, ".baidu.com") {
					parsed.Scheme = "https"
					return parsed.String()
				}
			}
		}
		return s
	case strings.HasPrefix(s, "//"):
		return scheme + ":" + s
	case strings.HasPrefix(s, "/"):
		return strings.TrimRight(passportBase, "/") + s
	default:
		return scheme + "://" + s
	}
}

func unwrapJSONP(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	trimmed = bytes.TrimRight(trimmed, "; \t\r\n")
	if len(trimmed) == 0 {
		return trimmed
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return trimmed
	}
	openIdx := bytes.IndexByte(trimmed, '(')
	closeIdx := bytes.LastIndexByte(trimmed, ')')
	if openIdx >= 0 && closeIdx > openIdx {
		return bytes.TrimSpace(trimmed[openIdx+1 : closeIdx])
	}
	return trimmed
}

func generateQRGID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		now := uint64(time.Now().UnixNano())
		for i := 0; i < 8; i++ {
			b[i] = byte(now >> (i * 8))
			b[i+8] = byte((now * 6364136223846793005) >> (i * 8))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08X-%04X-%04X-%04X-%012X",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func writePrivateQRFile(targetPath string, data []byte) error {
	dir := filepath.Dir(targetPath)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dir, ".qrcode-*.png.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	_ = tmpFile.Chmod(0600)
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	cleanupTmp = false
	_ = os.Chmod(targetPath, 0600)
	return nil
}

// DecodeQRPNGMatrix 解码二维码 PNG 字节流并提取布尔模块矩阵 (true=深色模块, false=浅色模块)。
func DecodeQRPNGMatrix(pngBytes []byte) ([][]bool, error) {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("PNG 解码失败: %w", err)
	}
	return DecodeQRImageMatrix(img)
}

func isDeletedOrExpiredCookie(ck *http.Cookie) bool {
	if ck == nil {
		return true
	}
	val := strings.TrimSpace(ck.Value)
	if val == "" || strings.EqualFold(val, "deleted") {
		return true
	}
	if ck.MaxAge < 0 {
		return true
	}
	if !ck.Expires.IsZero() && ck.Expires.Before(time.Now().Add(-1*time.Minute)) {
		return true
	}
	return false
}

func extractNetdiskStokenFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	decoded := unescapePercentHex(string(body))
	if m := netdiskStokenRe.FindStringSubmatch(decoded); len(m) >= 2 {
		return m[1]
	}
	if m := netdiskStokenRe.FindSubmatch(body); len(m) >= 2 {
		return string(m[1])
	}
	return ""
}

func unescapePercentHex(s string) string {
	if strings.IndexByte(s, '%') < 0 {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi, ok1 := fromHexChar(s[i+1])
			lo, ok2 := fromHexChar(s[i+2])
			if ok1 && ok2 {
				out.WriteByte((hi << 4) | lo)
				i += 2
				continue
			}
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

func fromHexChar(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}

func writerTTYCols(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok || !ui.IsTTY(f) {
		return 0
	}
	cols, _, err := term.GetSize(int(f.Fd()))
	if err != nil || cols <= 0 {
		return 0
	}
	return cols
}

// DecodeQRImageMatrix 自动检测外围静区白边（及可选外框描边）与模块像素步长，提取标准 QR 布尔矩阵。
func DecodeQRImageMatrix(img image.Image) ([][]bool, error) {
	if img == nil {
		return nil, errors.New("二维码图像为空")
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, errors.New("二维码图像尺寸无效")
	}
	if width > 4096 || height > 4096 {
		return nil, errors.New("二维码图像尺寸超出安全上限")
	}

	dark := make([]bool, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			alpha := float64(a) / 65535.0
			rf := float64(r)*alpha + 65535.0*(1.0-alpha)
			gf := float64(g)*alpha + 65535.0*(1.0-alpha)
			bf := float64(b)*alpha + 65535.0*(1.0-alpha)
			lum := 0.299*rf + 0.587*gf + 0.114*bf
			if lum < 32768.0 {
				dark[y*width+x] = true
			}
		}
	}

	matrix, minX, minY, maxX, maxY, err := decodeQRSubregion(dark, width, height, 0, 0, width-1, height-1)
	if err == nil {
		return matrix, nil
	}

	// 若图片外侧带有细深色矩形边框（外框与内部二维码之间隔有静区白边），剥离外框后重试一次
	if maxX >= minX && maxY >= minY {
		boxW := maxX - minX + 1
		boxH := maxY - minY + 1
		maxInset := boxW / 6
		if boxH/6 < maxInset {
			maxInset = boxH / 6
		}
		if maxInset > 32 {
			maxInset = 32
		}
		for inset := 1; inset <= maxInset; inset++ {
			if perimeterDarkRatio(dark, width, minX+inset, minY+inset, maxX-inset, maxY-inset) < 0.10 {
				if innerMat, _, _, _, _, innerErr := decodeQRSubregion(dark, width, height, minX+inset, minY+inset, maxX-inset, maxY-inset); innerErr == nil {
					return innerMat, nil
				}
				break
			}
		}
	}

	return nil, err
}

func perimeterDarkRatio(dark []bool, width, x0, y0, x1, y1 int) float64 {
	if x1-x0 < 20 || y1-y0 < 20 {
		return 1.0
	}
	darkCount, total := 0, 0
	for x := x0; x <= x1; x++ {
		total += 2
		if dark[y0*width+x] {
			darkCount++
		}
		if dark[y1*width+x] {
			darkCount++
		}
	}
	for y := y0 + 1; y < y1; y++ {
		total += 2
		if dark[y*width+x0] {
			darkCount++
		}
		if dark[y*width+x1] {
			darkCount++
		}
	}
	if total == 0 {
		return 1.0
	}
	return float64(darkCount) / float64(total)
}

func decodeQRSubregion(dark []bool, width, height, bx0, by0, bx1, by1 int) ([][]bool, int, int, int, int, error) {
	minX, minY := width, height
	maxX, maxY := -1, -1
	for y := by0; y <= by1; y++ {
		rowOff := y * width
		for x := bx0; x <= bx1; x++ {
			if dark[rowOff+x] {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}

	if maxX < minX || maxY < minY {
		return nil, minX, minY, maxX, maxY, errors.New("二维码图像不包含有效深色模块")
	}

	qrW := maxX - minX + 1
	qrH := maxY - minY + 1
	if qrW < 21 || qrH < 21 {
		return nil, minX, minY, maxX, maxY, errors.New("二维码有效区域尺寸小于最小标准矩阵 (21x21)")
	}
	aspect := float64(qrW) / float64(qrH)
	if aspect < 0.8 || aspect > 1.25 {
		return nil, minX, minY, maxX, maxY, errors.New("二维码有效区域非正方形")
	}

	sampleAt := func(n, col, row int) bool {
		stepX := float64(qrW) / float64(n)
		stepY := float64(qrH) / float64(n)
		cx := minX + int((float64(col)+0.5)*stepX)
		cy := minY + int((float64(row)+0.5)*stepY)
		if cx < minX {
			cx = minX
		} else if cx > maxX {
			cx = maxX
		}
		if cy < minY {
			cy = minY
		} else if cy > maxY {
			cy = maxY
		}
		if stepX >= 3.0 && stepY >= 3.0 {
			votes := 0
			for dy := -1; dy <= 1; dy++ {
				py := cy + dy
				if py < 0 {
					py = 0
				} else if py >= height {
					py = height - 1
				}
				for dx := -1; dx <= 1; dx++ {
					px := cx + dx
					if px < 0 {
						px = 0
					} else if px >= width {
						px = width - 1
					}
					if dark[py*width+px] {
						votes++
					}
				}
			}
			return votes >= 5
		}
		return dark[cy*width+cx]
	}

	// 扫描左上角定位符顶部若干像素行估算 7 模块连续深色宽度，兼容圆角/抗锯齿边缘
	maxTopRun := 0
	maxScanY := 8
	if qrH/3 < maxScanY {
		maxScanY = qrH / 3
	}
	maxLeadGap := qrW / 6
	if maxLeadGap < 4 {
		maxLeadGap = 4
	}
	for dy := 0; dy < maxScanY && minY+dy <= maxY; dy++ {
		rowOff := (minY + dy) * width
		sx := minX
		for sx <= maxX && sx < minX+maxLeadGap && !dark[rowOff+sx] {
			sx++
		}
		run := 0
		for x := sx; x <= maxX && dark[rowOff+x]; x++ {
			run++
		}
		if run > maxTopRun {
			maxTopRun = run
		}
	}
	estStride := float64(maxTopRun) / 7.0

	bestN := 0
	bestScore := -1.0
	bestStrideDiff := math.MaxFloat64

	for k := 0; k < 40; k++ {
		n := 21 + 4*k
		if n > qrW || n > qrH {
			break
		}
		score := scoreQRStructure(n, sampleAt)
		stride := float64(qrW) / float64(n)
		strideDiff := math.Abs(stride - estStride)
		if score > bestScore+1e-9 || (math.Abs(score-bestScore) <= 1e-9 && strideDiff < bestStrideDiff) {
			bestScore = score
			bestN = n
			bestStrideDiff = strideDiff
		}
	}

	if bestN == 0 || bestScore < 0.95 {
		return nil, minX, minY, maxX, maxY, errors.New("未能识别有效的标准二维码定位符结构")
	}

	matrix := make([][]bool, bestN)
	for r := 0; r < bestN; r++ {
		row := make([]bool, bestN)
		for c := 0; c < bestN; c++ {
			row[c] = sampleAt(bestN, c, r)
		}
		matrix[r] = row
	}
	return matrix, minX, minY, maxX, maxY, nil
}

func scoreQRStructure(n int, sampleAt func(n, col, row int) bool) float64 {
	matched, total := 0, 0
	check := func(col, row int, wantDark bool) {
		total++
		if sampleAt(n, col, row) == wantDark {
			matched++
		}
	}

	// 三个 7x7 定位图案 (左上、右上、左下)
	origins := [][2]int{{0, 0}, {n - 7, 0}, {0, n - 7}}
	for _, org := range origins {
		ox, oy := org[0], org[1]
		for dy := 0; dy < 7; dy++ {
			for dx := 0; dx < 7; dx++ {
				want := dx == 0 || dx == 6 || dy == 0 || dy == 6 || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4)
				check(ox+dx, oy+dy, want)
			}
		}
	}

	// 三个定位图案外侧 1 模块宽浅色分隔带
	for i := 0; i < 8; i++ {
		check(7, i, false)
		if i < 7 {
			check(i, 7, false)
		}
		check(n-8, i, false)
		if i < 7 {
			check(n-7+i, 7, false)
		}
		check(7, n-8+i, false)
		if i < 7 {
			check(i, n-8, false)
		}
	}

	// 第 6 行与第 6 列的黑白交替时序图案
	for idx := 8; idx <= n-9; idx++ {
		want := idx%2 == 0
		check(idx, 6, want)
		check(6, idx, want)
	}

	if total == 0 {
		return 0
	}
	return float64(matched) / float64(total)
}

// RenderQRMatrixDense1x2 将布尔二维码矩阵渲染为带外围静区的 Dense1x2 Unicode 半块字符画。
// 开启色彩时使用显式白底黑字 (#000000 on #FFFFFF) 确保深色与浅色终端主题下均能高对比度稳定识别；
// --no-color 下输出纯 Unicode 半块；--ascii 或物理终端列宽不足以容纳矩阵时返回空字符串交由上层提示本地 PNG 路径。
func RenderQRMatrixDense1x2(w io.Writer, matrix [][]bool, colorOn, asciiOnly bool) string {
	if asciiOnly || len(matrix) == 0 || len(matrix[0]) == 0 {
		return ""
	}
	rows := len(matrix)
	cols := len(matrix[0])
	quietZone := 2
	if termCols := writerTTYCols(w); termCols > 0 && cols+2*quietZone > termCols {
		if cols+2 <= termCols {
			quietZone = 1
		} else {
			return ""
		}
	}

	totalRows := rows + 2*quietZone
	totalCols := cols + 2*quietZone

	darkAt := func(r, c int) bool {
		mr := r - quietZone
		mc := c - quietZone
		if mr < 0 || mr >= rows || mc < 0 || mc >= len(matrix[mr]) {
			return false
		}
		return matrix[mr][mc]
	}

	var rowStyle lipgloss.Style
	if colorOn {
		r := ui.NewRenderer(w, &colorOn)
		rowStyle = r.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#FFFFFF"))
	}

	var lines []string
	for r := 0; r < totalRows; r += 2 {
		var lineBuf strings.Builder
		lineBuf.Grow(totalCols * 3)
		for c := 0; c < totalCols; c++ {
			top := darkAt(r, c)
			bot := false
			if r+1 < totalRows {
				bot = darkAt(r+1, c)
			}
			switch {
			case top && bot:
				lineBuf.WriteRune('█')
			case top && !bot:
				lineBuf.WriteRune('▀')
			case !top && bot:
				lineBuf.WriteRune('▄')
			default:
				lineBuf.WriteByte(' ')
			}
		}
		rawLine := lineBuf.String()
		if colorOn {
			lines = append(lines, rowStyle.Render(rawLine))
		} else {
			lines = append(lines, rawLine)
		}
	}
	return strings.Join(lines, "\n")
}
