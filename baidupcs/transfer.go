package baidupcs

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
	"github.com/tidwall/gjson"
)

var shareLoginStateRe = regexp.MustCompile(`(\{.+?loginstate.+?\})`)

type (
	// TransferOption 分享转存可选项
	TransferOption struct {
		Download bool   // 是否直接开始下载
		Collect  bool   // 多文件整合
		FsID     string // 指定 fs_id 转存
	}

	// SharePageTokens 分享页面提取出的会话令牌与元信息
	SharePageTokens struct {
		BDSToken string
		UK       string
		ShareUK  string
		ShareID  string
	}

	// ShareTransferMeta 分享项列表解析出的待转存元信息
	ShareTransferMeta struct {
		Filename     string
		ShareID      string
		BDSToken     string
		FsIDListJSON string
		ItemNum      int
		TransferURL  string
	}

	// ShareTransferRequest 提交转存请求的参数
	ShareTransferRequest struct {
		TransferURL  string
		Referer      string
		FsIDListJSON string
		TargetPath   string
	}

	// ShareTransferResult 转存成功后的文件信息
	ShareTransferResult struct {
		Filename  string
		Filenames []string
	}
)

// GenerateShareQueryURL 构造网盘分享子接口 URL
func (pcs *BaiduPCS) GenerateShareQueryURL(subPath string, params map[string]string) *url.URL {
	shareURL := &url.URL{
		Scheme: GetHTTPScheme(true),
		Host:   PanBaiduCom,
		Path:   "/share/" + subPath,
	}
	uv := shareURL.Query()
	for key, value := range params {
		uv.Set(key, value)
	}
	shareURL.RawQuery = uv.Encode()
	return shareURL
}

// AccessSharePage 访问分享页面并提取登录态 token 与分享者标识
func (pcs *BaiduPCS) AccessSharePage(featureStr string, first bool) (*SharePageTokens, error) {
	headers := map[string]string{
		"User-Agent": NetdiskUA, // 修 #534：netdisk 客户端 UA 放行获取真实分享页
		"Referer":    "https://pan.baidu.com/disk/home",
	}
	if !first && len(featureStr) > 1 {
		headers["Referer"] = fmt.Sprintf("https://pan.baidu.com/share/init?surl=%s", featureStr[1:])
	}
	shareLink := fmt.Sprintf("https://pan.baidu.com/s/%s", featureStr)

	dataReadCloser, panError := pcs.sendReqReturnReadCloser(reqTypePan, OperationShareFileSavetoLocal, http.MethodGet, shareLink, nil, headers)
	if panError != nil {
		return nil, errors.New("访问分享页失败")
	}
	defer dataReadCloser.Close()

	body, err := io.ReadAll(dataReadCloser)
	if err != nil {
		return nil, fmt.Errorf("读取分享页响应失败: %w", err)
	}
	bodyStr := string(body)
	if strings.Contains(bodyStr, "error-404") {
		return nil, errors.New("页面不存在")
	}
	if strings.Contains(bodyStr, "platform-non-found") {
		return nil, errors.New("分享链接已失效")
	}

	sub := shareLoginStateRe.FindSubmatch(body)
	if len(sub) < 2 {
		return nil, errors.New("请确认登录参数中已经包含了网盘STOKEN")
	}
	rawJSON := string(sub[1])
	return &SharePageTokens{
		BDSToken: gjson.Get(rawJSON, `bdstoken`).String(),
		UK:       gjson.Get(rawJSON, `uk`).String(),
		ShareUK:  gjson.Get(rawJSON, `share_uk`).String(),
		ShareID:  gjson.Get(rawJSON, `shareid`).String(),
	}, nil
}

// PostShareVerify 提交分享提取码验证请求，返回 randsk
func (pcs *BaiduPCS) PostShareVerify(verifyURL, referer string, data map[string]string) (string, error) {
	dataReadCloser, panError := pcs.sendReqReturnReadCloser(reqTypePan, OperationShareFileSavetoLocal, http.MethodPost, verifyURL, data, map[string]string{
		"User-Agent":   requester.UserAgent,
		"Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
		"Referer":      referer,
	})
	if panError != nil {
		return "", errors.New("提交分享项查询请求时发生错误")
	}
	defer dataReadCloser.Close()

	body, err := io.ReadAll(dataReadCloser)
	if err != nil {
		return "", fmt.Errorf("读取提取码验证响应失败: %w", err)
	}
	errno := gjson.GetBytes(body, `errno`).Int()
	if errno != 0 {
		if errno == -9 {
			return "", errors.New("提取码错误")
		}
		return "", fmt.Errorf("未知错误, 错误码%d", errno)
	}
	return gjson.GetBytes(body, `randsk`).String(), nil
}

// ExtractShareInfo 查询分享列表并构造转存元信息
func (pcs *BaiduPCS) ExtractShareInfo(shareListURL, shareID, shareUK, bdstoken string) (*ShareTransferMeta, error) {
	dataReadCloser, panError := pcs.sendReqReturnReadCloser(reqTypePan, OperationShareFileSavetoLocal, http.MethodGet, shareListURL, nil, map[string]string{
		"User-Agent":   requester.UserAgent,
		"Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
	})
	if panError != nil {
		return nil, errors.New("提交分享项查询请求时发生错误")
	}
	defer dataReadCloser.Close()

	body, err := io.ReadAll(dataReadCloser)
	if err != nil {
		return nil, fmt.Errorf("读取分享列表失败: %w", err)
	}
	errno := gjson.GetBytes(body, `errno`).Int()
	if errno != 0 {
		if errno == 8001 {
			return nil, errors.New("已触发验证, 请稍后再试")
		}
		return nil, fmt.Errorf("未知错误, 错误码%d", errno)
	}

	filename := gjson.GetBytes(body, `list.0.server_filename`).String()
	fsidResults := gjson.GetBytes(body, `list.#.fs_id`).Array()
	fids := make([]string, 0, len(fsidResults))
	for _, sid := range fsidResults {
		fids = append(fids, sid.String())
	}

	transferURL := &url.URL{
		Scheme: GetHTTPScheme(true),
		Host:   PanBaiduCom,
		Path:   "/share/transfer",
	}
	uv := transferURL.Query()
	uv.Set("app_id", PanAppID)
	uv.Set("channel", "chunlei")
	uv.Set("clienttype", "0")
	uv.Set("web", "1")
	uv.Set("filename", filename)
	uv.Set("shareid", shareID)
	uv.Set("from", shareUK)
	uv.Set("bdstoken", bdstoken)
	transferURL.RawQuery = uv.Encode()

	return &ShareTransferMeta{
		Filename:     filename,
		ShareID:      shareID,
		BDSToken:     bdstoken,
		FsIDListJSON: "[" + strings.Join(fids, ",") + "]",
		ItemNum:      len(fsidResults),
		TransferURL:  transferURL.String(),
	}, nil
}

// ExecuteShareTransfer 执行网盘分享转存 POST 请求
func (pcs *BaiduPCS) ExecuteShareTransfer(req ShareTransferRequest) (*ShareTransferResult, error) {
	headers := map[string]string{
		"User-Agent":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/76.0.3809.100 Safari/537.36",
		"Referer":      req.Referer,
		"Content-Type": "application/x-www-form-urlencoded",
	}
	postData := map[string]string{
		"fsidlist": req.FsIDListJSON,
		"path":     req.TargetPath,
	}
	dataReadCloser, panError := pcs.sendReqReturnReadCloser(reqTypePan, OperationShareFileSavetoLocal, http.MethodPost, req.TransferURL, postData, headers)
	if panError != nil {
		return nil, errors.New("网络错误")
	}
	defer dataReadCloser.Close()

	body, err := io.ReadAll(dataReadCloser)
	if err != nil {
		return nil, errors.New("未知错误")
	}
	if !gjson.ValidBytes(body) {
		return nil, errors.New("返回json解析错误")
	}

	errno := gjson.GetBytes(body, `errno`).Int()
	if errno != 0 {
		if errno == 12 {
			infoPath := gjson.GetBytes(body, `info.0.path`).String()
			_, file := path.Split(infoPath)
			subErrno := gjson.GetBytes(body, `info.0.errno`).Int()
			targetFileNums := gjson.GetBytes(body, `target_file_nums`).Int()
			targetFileNumsLimit := gjson.GetBytes(body, `target_file_nums_limit`).Int()
			if targetFileNums > targetFileNumsLimit {
				return nil, fmt.Errorf("转存文件数%d超过当前用户上限, 当前用户单次最大转存数%d", targetFileNums, targetFileNumsLimit)
			}
			if subErrno == -30 {
				return nil, fmt.Errorf("当前目录下已有%s同名文件/文件夹", file)
			}
			return nil, fmt.Errorf("未知错误, 错误代码%d", subErrno)
		}
		if errno == 4 {
			return nil, errors.New("文件重复")
		}
		return nil, errors.New("获取分享项元数据错误")
	}

	_, firstFilename := path.Split(gjson.GetBytes(body, `info.0.path`).String())
	rawPaths := gjson.GetBytes(body, `info.#.path`).Array()
	names := make([]string, 0, len(rawPaths))
	for _, p := range rawPaths {
		names = append(names, path.Base(p.String()))
	}
	if len(names) == 0 {
		names = []string{"default" + pcsutil.GenerateRandomString(5)}
	}
	if len(gjson.GetBytes(body, `info.#.fsid`).Array()) > 1 {
		firstFilename += "等多个文件/文件夹"
	}

	pcs.deleteCache([]string{req.TargetPath, path.Dir(req.TargetPath)})

	return &ShareTransferResult{
		Filename:  firstFilename,
		Filenames: names,
	}, nil
}
