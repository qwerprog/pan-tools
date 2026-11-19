package pcsconfig

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
)

const (
	opDelete = "delete"
	opSwitch = "switch"
	opGet    = "get"
)

func matchesBaiduBase(user *Baidu, target *BaiduBase) bool {
	if user == nil || target == nil {
		return false
	}
	switch {
	case target.UID != 0 && target.Name != "":
		return user.UID == target.UID && strings.EqualFold(user.Name, target.Name)
	case target.UID == 0 && target.Name != "":
		return strings.EqualFold(user.Name, target.Name)
	case target.UID != 0 && target.Name == "":
		return user.UID == target.UID
	default:
		return false
	}
}

func (c *PCSConfig) manipUser(op string, baiduBase *BaiduBase) (*Baidu, error) {
	if baiduBase == nil || (baiduBase.UID == 0 && baiduBase.Name == "") {
		if op == opGet {
			return &Baidu{}, nil
		}
		return nil, ErrBaiduUserNotFound
	}
	if len(c.BaiduUserList) == 0 {
		return nil, ErrNoSuchBaiduUser
	}

	for k, user := range c.BaiduUserList {
		if !matchesBaiduBase(user, baiduBase) {
			continue
		}

		switch op {
		case opSwitch:
			c.setupNewUser(user)
		case opDelete:
			c.BaiduUserList = append(c.BaiduUserList[:k], c.BaiduUserList[k+1:]...)
			if c.BaiduActiveUID == user.UID {
				if len(c.BaiduUserList) != 0 {
					c.setupNewUser(c.BaiduUserList[0])
				} else {
					c.BaiduActiveUID = 0
					c.activeUser = nil
					c.pcs = nil
				}
			}
		case opGet:
			// 仅查询
		}
		return user, nil
	}

	return nil, ErrBaiduUserNotFound
}

// setupNewUser 从已有用户中, 设置新的当前登录用户
func (c *PCSConfig) setupNewUser(user *Baidu) {
	if user == nil {
		return
	}
	c.BaiduActiveUID = user.UID
	c.activeUser = user
	c.pcs = c.createBaiduPCS(user)
}

// SwitchUser 切换用户, 返回切换成功的用户
func (c *PCSConfig) SwitchUser(baiduBase *BaiduBase) (*Baidu, error) {
	return c.manipUser(opSwitch, baiduBase)
}

// DeleteUser 删除用户, 返回删除成功的用户
func (c *PCSConfig) DeleteUser(baiduBase *BaiduBase) (*Baidu, error) {
	return c.manipUser(opDelete, baiduBase)
}

// GetBaiduUser 获取百度用户信息
func (c *PCSConfig) GetBaiduUser(baidubase *BaiduBase) (*Baidu, error) {
	return c.manipUser(opGet, baidubase)
}

// CheckBaiduUserExist 检查百度用户是否存在于已登录列表
func (c *PCSConfig) CheckBaiduUserExist(baidubase *BaiduBase) bool {
	_, err := c.manipUser("", baidubase)
	return err == nil
}

var cookieHeaderPrefixRe = regexp.MustCompile(`(?i)^cookies?\s*:\s*`)

// NormalizeCookieHeader 清理用户从浏览器开发者工具直接复制的 Cookie(s): 前缀与首尾空白。
func NormalizeCookieHeader(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	return strings.TrimSpace(cookieHeaderPrefixRe.ReplaceAllString(s, ""))
}

// SetupUserByBDUSS 设置百度 bduss, ptoken, stoken, cookies 并保存
func (c *PCSConfig) SetupUserByBDUSS(bduss, ptoken, stoken, cookies string) (baidu *Baidu, err error) {
	cookies = NormalizeCookieHeader(cookies)
	bduss = strings.TrimSpace(bduss)
	ptoken = strings.TrimSpace(ptoken)
	stoken = strings.TrimSpace(stoken)

	var cookieBDUSS, cookiePTOKEN, cookieSTOKEN, cookieBAIDUID, cookieSBOXTKN string
	if cookies != "" {
		for _, ck := range requester.ParseCookieStr(cookies) {
			if ck.Value == "" {
				continue
			}
			switch ck.Name {
			case "BDUSS":
				cookieBDUSS = ck.Value
			case "PTOKEN":
				cookiePTOKEN = ck.Value
			case "STOKEN":
				cookieSTOKEN = ck.Value
			case "BAIDUID":
				cookieBAIDUID = ck.Value
			case "SBOXTKN":
				cookieSBOXTKN = ck.Value
			}
		}
		if bduss == "" {
			if cookieBDUSS == "" {
				return nil, errors.New("cookies 中未找到有效的 BDUSS")
			}
			bduss = cookieBDUSS
		}
		if ptoken == "" {
			ptoken = cookiePTOKEN
		}
		if stoken == "" {
			stoken = cookieSTOKEN
		}
	}
	if bduss == "" {
		return nil, errors.New("BDUSS 不能为空")
	}

	var b *Baidu
	if c.ForceLogin != "" {
		b, err = NewUserInfoByInput(bduss, c.ForceLogin)
	} else {
		b, err = c.newUserInfoByNetdisk(bduss, stoken, cookies)
		if err != nil {
			return nil, fmt.Errorf("网盘用户信息校验失败: %w", err)
		}
	}
	if err != nil {
		return nil, err
	}

	c.DeleteUser(&BaiduBase{
		UID: b.UID,
	}) // 删除旧的信息

	b.BDUSS = bduss
	b.PTOKEN = ptoken // 实际未使用
	b.STOKEN = stoken
	if cookieBAIDUID != "" {
		b.BAIDUID = cookieBAIDUID
	}
	if cookieSBOXTKN != "" {
		b.SBOXTKN = cookieSBOXTKN
	}
	b.COOKIES = cookies

	c.BaiduUserList = append(c.BaiduUserList, b)

	// 自动切换用户
	c.setupNewUser(b)
	return b, nil
}

// SetAppID 设置app_id
func (c *PCSConfig) SetAppID(appID int) {
	c.AppID = appID
	if c.pcs != nil {
		c.pcs.SetAPPID(appID)
	}
}

// SetCacheSizeByStr 设置cache_size
func (c *PCSConfig) SetCacheSizeByStr(sizeStr string) error {
	size, err := converter.ParseFileSizeStr(sizeStr)
	if err != nil {
		return err
	}
	c.CacheSize = int(size)
	return nil
}

// SetMaxDownloadRateByStr 设置 max_download_rate
func (c *PCSConfig) SetMaxDownloadRateByStr(sizeStr string) error {
	size, err := converter.ParseFileSizeStr(stripPerSecond(sizeStr))
	if err != nil {
		return err
	}
	c.MaxDownloadRate = size
	return nil
}

// SetMaxUploadRateByStr 设置 max_upload_rate
func (c *PCSConfig) SetMaxUploadRateByStr(sizeStr string) error {
	size, err := converter.ParseFileSizeStr(stripPerSecond(sizeStr))
	if err != nil {
		return err
	}
	c.MaxUploadRate = size
	return nil
}

// SetUserAgent 设置User-Agent
func (c *PCSConfig) SetUserAgent(userAgent string) {
	c.UserAgent = userAgent
	requester.UserAgent = userAgent
}

// SetPCSUA 设置 PCS User-Agent
func (c *PCSConfig) SetPCSUA(pcsUA string) {
	c.PCSUA = pcsUA
	if c.pcs != nil {
		c.pcs.SetPCSUserAgent(pcsUA)
	}
}

// SetPanUA 设置 Pan User-Agent
func (c *PCSConfig) SetPanUA(panUA string) {
	c.PanUA = panUA
	if c.pcs != nil {
		c.pcs.SetPanUserAgent(panUA)
	}
}

// SetPCSAddr 设置 PCS 服务器地址
func (c *PCSConfig) SetPCSAddr(pcsaddr string) bool {
	match, _ := regexp.MatchString("^([cd]\\d?\\.)?pcs\\.baidu\\.com", pcsaddr)
	if match {
		c.PCSAddr = pcsaddr
		if c.pcs != nil {
			c.pcs.SetPCSAddr(pcsaddr)
		}
	}
	return match
}

// SetStaticPCSAddr 设置上传时是否关闭动态PCS域名
func (c *PCSConfig) SetStaticPCSAddr(static bool) {
	c.FixPCSAddr = static
	if c.pcs != nil {
		c.pcs.SetStaticPCSAddr(static)
	}
}

// SetEnableHTTPS 设置是否启用https
func (c *PCSConfig) SetEnableHTTPS(https bool) {
	c.EnableHTTPS = https
	if c.pcs != nil {
		c.pcs.SetHTTPS(https)
	}
}

func (c *PCSConfig) SetNoCheck(nocheck bool) {
	c.NoCheck = nocheck
}

// SetUploadPolicy 设置上传文件重名时的处理策略
func (c *PCSConfig) SetUploadPolicy(upolicy string) {
	c.UPolicy = upolicy
}

// SetProxy 设置代理
func (c *PCSConfig) SetProxy(proxy string) {
	c.Proxy = proxy
	requester.SetGlobalProxy(proxy)
}

// SetProxyHostnames 设置代理域名列表
func (c *PCSConfig) SetProxyHostnames(proxyHostnames string) {
	c.ProxyHostnames = proxyHostnames
	requester.SetProxyHostnameRules(proxyHostnames)
}

// SetLocalAddrs 设置localAddrs
func (c *PCSConfig) SetLocalAddrs(localAddrs string) {
	c.LocalAddrs = localAddrs
	requester.SetLocalTCPAddrList(strings.Split(localAddrs, ",")...)
}

// SetIgnoreIllegal 设置忽略上传文件名非法字符
func (c *PCSConfig) SetIgnoreIllegal(ignore bool) {
	c.IgnoreIllegal = ignore
}

// SetForceLogin 设置强制登录
func (c *PCSConfig) SetForceLogin(username string) {
	c.ForceLogin = username
}

// SetPromptStyle 设置交互式 REPL 提示符风格 (starship / boxed)
func (c *PCSConfig) SetPromptStyle(style string) error {
	parsed, err := ui.ParsePromptStyle(style)
	if err != nil {
		return err
	}
	c.PromptStyle = string(parsed)
	return nil
}
