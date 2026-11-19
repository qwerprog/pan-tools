package pcsconfig

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
)

var (
	//ErrNoSuchBaiduUser 未登录任何百度帐号
	ErrNoSuchBaiduUser = errors.New("no such baidu user")
	//ErrBaiduUserNotFound 未找到百度帐号
	ErrBaiduUserNotFound = errors.New("baidu user not found")

	netdiskPanBaseURL = "https://pan.baidu.com"
)

// OverrideNetdiskBaseURLForTest 供跨包端到端测试替换网盘用户信息接口基址，返回恢复闭包。
func OverrideNetdiskBaseURLForTest(panBaseURL string) func() {
	origPan := netdiskPanBaseURL
	if panBaseURL != "" {
		netdiskPanBaseURL = panBaseURL
	}
	return func() {
		netdiskPanBaseURL = origPan
	}
}

// BaiduBase Baidu基
type BaiduBase struct {
	UID  uint64 `json:"uid"`  // 百度ID对应的uid
	Name string `json:"name"` // 真实ID
}

// Baidu 百度帐号对象
type Baidu struct {
	BaiduBase
	Sex string  `json:"sex"` // 性别
	Age float64 `json:"age"` // 帐号年龄

	BDUSS   string `json:"bduss"`
	PTOKEN  string `json:"ptoken"`
	STOKEN  string `json:"stoken"`
	BAIDUID string `json:"baiduid"`
	SBOXTKN string `json:"sboxtkn"`
	COOKIES string `json:"cookies"`

	Workdir string `json:"workdir"` // 工作目录
}

func (c *PCSConfig) createBaiduPCS(baidu *Baidu) *baidupcs.BaiduPCS {
	if baidu == nil {
		baidu = &Baidu{}
	}
	var pcs *baidupcs.BaiduPCS
	if strings.TrimSpace(baidu.COOKIES) != "" {
		pcs = baidupcs.NewPCSWithCookieStr(c.AppID, baidu.COOKIES)
		if baidu.BDUSS != "" {
			pcs.SetBDUSS(baidu.BDUSS)
		}
		if baidu.STOKEN != "" {
			pcs.SetStoken(baidu.STOKEN)
		}
	} else {
		pcs = baidupcs.NewPCS(c.AppID, baidu.BDUSS)
		pcs.SetStoken(baidu.STOKEN)
	}
	if baidu.SBOXTKN != "" {
		pcs.SetSboxtkn(baidu.SBOXTKN)
	}
	pcs.SetHTTPS(c.EnableHTTPS)
	pcs.SetPCSUserAgent(c.PCSUA)
	pcs.SetPanUserAgent(c.PanUA)
	pcs.SetPCSAddr(c.PCSAddr)
	pcs.SetStaticPCSAddr(c.FixPCSAddr)
	pcs.SetUID(baidu.UID)
	return pcs
}

// BaiduPCS 初始化*baidupcs.BaiduPCS
func (baidu *Baidu) BaiduPCS() *baidupcs.BaiduPCS {
	return Config.createBaiduPCS(baidu)
}

// GetSavePath 根据提供的网盘文件路径 pcspath, 返回本地储存路径,
// 返回绝对路径, 获取绝对路径出错时才返回相对路径...
func (baidu *Baidu) GetSavePath(pcspath string) string {
	dirStr := filepath.Join(Config.SaveDir, fmt.Sprintf("%d_%s", baidu.UID, converter.TrimPathInvalidChars(baidu.Name)), pcspath)
	dir, err := filepath.Abs(dirStr)
	if err != nil {
		dir = filepath.Clean(dirStr)
	}
	return dir
}

// PathJoin 合并工作目录和相对路径p, 若p为绝对路径则忽略
func (baidu *Baidu) PathJoin(p string) string {
	if path.IsAbs(p) {
		return p
	}
	return path.Join(baidu.Workdir, p)
}

// BaiduUserList 百度帐号列表
type BaiduUserList []*Baidu

// NewUserInfoByBDUSS 通过 HTTPS 百度网盘接口检测 BDUSS 有效性并获取基础用户信息 (无法获取 ptoken 和 stoken)
func NewUserInfoByBDUSS(bduss string) (b *Baidu, err error) {
	return (&PCSConfig{}).newUserInfoByNetdisk(bduss, "", "")
}

type netdiskXpanUinfoJSON struct {
	Errno       int    `json:"errno"`
	Errmsg      string `json:"errmsg"`
	BaiduName   string `json:"baidu_name"`
	NetdiskName string `json:"netdisk_name"`
	UK          uint64 `json:"uk"`
}

type netdiskGetInfoJSON struct {
	Errno   int `json:"errno"`
	Records []struct {
		UK        uint64 `json:"uk"`
		Uname     string `json:"uname"`
		BaiduName string `json:"baidu_name"`
		NickName  string `json:"nick_name"`
	} `json:"records"`
}

// newUserInfoByNetdisk 通过 HTTPS 百度网盘接口回退获取用户信息 (uk + 用户名)，
// 解决贴吧接口未开通、明文 HTTP 被拦截或限流导致有效网盘 BDUSS 登录失败的问题。
func (c *PCSConfig) newUserInfoByNetdisk(bduss, stoken, cookies string) (*Baidu, error) {
	baseRaw := strings.TrimRight(strings.TrimSpace(netdiskPanBaseURL), "/")
	if baseRaw == "" {
		baseRaw = "https://pan.baidu.com"
	}
	baseURL, err := url.Parse(baseRaw)
	if err != nil {
		return nil, fmt.Errorf("解析网盘接口地址失败: %w", err)
	}

	client := requester.NewHTTPClient()
	client.SetHTTPSecure(true)
	client.SetTLSHandshakeTimeout(5 * time.Second)
	client.SetTimeout(15 * time.Second)
	jar, _ := cookiejar.New(nil)

	cookieDomain := ""
	if host := strings.ToLower(baseURL.Hostname()); host == "pan.baidu.com" || strings.HasSuffix(host, ".baidu.com") {
		cookieDomain = baidupcs.DotBaiduCom
	}
	var jarCookies []*http.Cookie
	if strings.TrimSpace(cookies) != "" {
		for _, ck := range requester.ParseCookieStr(cookies) {
			if ck.Name == "" || ck.Value == "" {
				continue
			}
			ck.Domain = cookieDomain
			jarCookies = append(jarCookies, ck)
		}
	}
	if bduss != "" {
		jarCookies = append(jarCookies, &http.Cookie{Name: "BDUSS", Value: bduss, Domain: cookieDomain})
	}
	if stoken != "" {
		jarCookies = append(jarCookies, &http.Cookie{Name: "STOKEN", Value: stoken, Domain: cookieDomain})
	}
	jar.SetCookies(baseURL, jarCookies)
	client.SetCookiejar(jar)

	panUA := baidupcs.NetdiskUA
	if c != nil && strings.TrimSpace(c.PanUA) != "" {
		panUA = c.PanUA
	}
	headers := map[string]string{
		"User-Agent": panUA,
		"Referer":    "https://pan.baidu.com/disk/main",
	}

	// 1. 优先请求 /rest/2.0/xpan/nas?method=uinfo (仅 BDUSS 或 BDUSS+STOKEN 均可获取 uk 与 baidu_name/netdisk_name)
	uinfoURL := baseRaw + "/rest/2.0/xpan/nas?method=uinfo"
	uinfoErr := error(nil)
	if resp, reqErr := client.Req(http.MethodGet, uinfoURL, nil, headers); reqErr != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		client.CloseIdleConnections()
		requester.RotateTCPHostCache(baseURL.Hostname())
		uinfoErr = reqErr
	} else {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			client.CloseIdleConnections()
			requester.RotateTCPHostCache(baseURL.Hostname())
			uinfoErr = readErr
		} else if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout {
				client.CloseIdleConnections()
				requester.RotateTCPHostCache(baseURL.Hostname())
			}
			uinfoErr = fmt.Errorf("xpan uinfo HTTP %d", resp.StatusCode)
		} else {
			var parsed netdiskXpanUinfoJSON
			if jsonErr := jsoniter.Unmarshal(body, &parsed); jsonErr != nil {
				uinfoErr = jsonErr
			} else if parsed.Errno != 0 || parsed.UK == 0 {
				uinfoErr = fmt.Errorf("xpan uinfo errno=%d errmsg=%s", parsed.Errno, parsed.Errmsg)
			} else {
				name := strings.TrimSpace(parsed.BaiduName)
				if name == "" {
					name = strings.TrimSpace(parsed.NetdiskName)
				}
				if name == "" {
					name = fmt.Sprintf("uid_%d", parsed.UK)
				}
				return &Baidu{
					BaiduBase: BaiduBase{
						UID:  parsed.UK,
						Name: name,
					},
					Sex:     "unknown",
					Age:     0,
					BDUSS:   bduss,
					Workdir: "/",
				}, nil
			}
		}
	}

	// 2. 次选请求 /api/user/getinfo?need_selfinfo=1
	getInfoURL := baseRaw + "/api/user/getinfo?need_selfinfo=1"
	resp, reqErr := client.Req(http.MethodGet, getInfoURL, nil, headers)
	if reqErr != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("%v; getinfo: %w", uinfoErr, reqErr)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("%v; getinfo read: %w", uinfoErr, readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%v; getinfo HTTP %d", uinfoErr, resp.StatusCode)
	}
	var parsedInfo netdiskGetInfoJSON
	if jsonErr := jsoniter.Unmarshal(body, &parsedInfo); jsonErr != nil {
		return nil, fmt.Errorf("%v; getinfo json: %w", uinfoErr, jsonErr)
	}
	if parsedInfo.Errno != 0 || len(parsedInfo.Records) == 0 || parsedInfo.Records[0].UK == 0 {
		return nil, fmt.Errorf("%v; getinfo errno=%d", uinfoErr, parsedInfo.Errno)
	}
	rec := parsedInfo.Records[0]
	name := strings.TrimSpace(rec.Uname)
	if name == "" {
		name = strings.TrimSpace(rec.BaiduName)
	}
	if name == "" {
		name = strings.TrimSpace(rec.NickName)
	}
	if name == "" {
		name = fmt.Sprintf("uid_%d", rec.UK)
	}
	return &Baidu{
		BaiduBase: BaiduBase{
			UID:  rec.UK,
			Name: name,
		},
		Sex:     "unknown",
		Age:     0,
		BDUSS:   bduss,
		Workdir: "/",
	}, nil
}

// NewUserInfoByInput 不检测BDUSS有效性, 手动设置百度详细信息 (适用于网盘用户信息接口不可用的情况)
func NewUserInfoByInput(bduss, name string) (b *Baidu, err error) {
	b = &Baidu{
		BaiduBase: BaiduBase{
			UID:  24,
			Name: name,
		},
		Sex:     "default",
		Age:     24,
		BDUSS:   bduss,
		Workdir: "/",
	}
	return b, nil
}

// String 格式输出百度帐号列表
func (bl *BaiduUserList) String() string {
	builder := &strings.Builder{}
	colorOn := ui.ColorEnabled(os.Stdout)

	tb := pcstable.NewTable(builder)
	tb.SetColumnAlignment([]int{pcstable.AlignDefault, pcstable.AlignCenter, pcstable.AlignRight, pcstable.AlignLeft, pcstable.AlignCenter, pcstable.AlignRight, pcstable.AlignLeft})
	tb.SetHeader([]string{"#", "状态", "UID", "用户名", "性别", "帐龄", "工作目录"})

	activeUID := Config.BaiduActiveUID
	for k, baiduInfo := range *bl {
		if baiduInfo == nil {
			continue
		}
		status := ui.StyleMuted(os.Stdout, "-", colorOn)
		if activeUID != 0 && baiduInfo.UID == activeUID {
			status = ui.RenderBadge(os.Stdout, ui.BadgeActive, "当前", colorOn)
		}
		workdir := baiduInfo.Workdir
		if workdir == "" {
			workdir = "/"
		}
		tb.Append([]string{
			strconv.Itoa(k),
			status,
			ui.MaskUID(baiduInfo.UID),
			baiduInfo.Name,
			baiduInfo.Sex,
			fmt.Sprintf("%.1f", baiduInfo.Age),
			workdir,
		})
	}

	tb.Render()

	return builder.String()
}
