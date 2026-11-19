package pcscommand

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
)

var (
	shareLinkExtractRe         = regexp.MustCompile(`((?:https?://)?pan\.baidu\.com/(?:s/[0-9A-Za-z_-]+(?:\?[0-9A-Za-z_=&%-]*)?|share/init\?[0-9A-Za-z_=&%-]+))`)
	sharePwdTextRe             = regexp.MustCompile(`提取码\s*[:：]?\s*([0-9A-Za-z]{4})`)
	sharePwdCodeRe             = regexp.MustCompile(`^[0-9A-Za-z]{4}$`)
	shareTransferDownloadDelay = 10 * time.Second
)

func parseShareTransferArgs(params []string) (normalizedLink, featureStr, extraCode string, err error) {
	var nonEmpty []string
	for _, p := range params {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			nonEmpty = append(nonEmpty, trimmed)
		}
	}
	if len(nonEmpty) == 0 {
		return "", "", "", fmt.Errorf("%s失败: 未提供分享链接", baidupcs.OperationShareFileSavetoLocal)
	}

	fullText := strings.Join(nonEmpty, " ")
	if strings.Contains(fullText, "bdlink=") {
		return "", "", "", fmt.Errorf("%s失败: 秒传已不再被支持", baidupcs.OperationShareFileSavetoLocal)
	}

	link := nonEmpty[0]
	if m := shareLinkExtractRe.FindStringSubmatch(fullText); len(m) >= 2 {
		link = m[1]
	} else if !strings.Contains(link, "pan.baidu.com/") {
		return "", "", "", fmt.Errorf("%s失败: 秒传已不再被支持", baidupcs.OperationShareFileSavetoLocal)
	}

	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		link = "https://" + strings.TrimPrefix(link, "//")
	}

	parsedURL, parseErr := url.Parse(link)
	if parseErr != nil || parsedURL.Host == "" {
		return "", "", "", fmt.Errorf("%s失败: 链接格式非法", baidupcs.OperationShareFileSavetoLocal)
	}

	queryParams := parsedURL.Query()
	extraCode = strings.TrimSpace(queryParams.Get("pwd"))
	if extraCode == "" {
		if pm := sharePwdTextRe.FindStringSubmatch(fullText); len(pm) == 2 {
			extraCode = pm[1]
		} else if len(nonEmpty) == 2 && !strings.Contains(nonEmpty[1], "pan.baidu.com") {
			extraCode = nonEmpty[1]
		}
	}

	featureStr = path.Base(strings.TrimSuffix(parsedURL.Path, "/"))
	if featureStr == "init" {
		surl := strings.TrimSpace(queryParams.Get("surl"))
		if surl != "" {
			featureStr = "1" + surl
		}
	}

	if len(featureStr) <= 1 || len(featureStr) > 23 || featureStr[:1] != "1" {
		return "", "", "", fmt.Errorf("%s失败: 链接地址或提取码非法", baidupcs.OperationShareFileSavetoLocal)
	}
	if extraCode != "" && !sharePwdCodeRe.MatchString(extraCode) {
		return "", "", "", fmt.Errorf("%s失败: 链接地址或提取码非法", baidupcs.OperationShareFileSavetoLocal)
	}
	return link, featureStr, extraCode, nil
}

// RunShareTransfer 执行分享链接转存到网盘
func RunShareTransfer(params []string, opt *baidupcs.TransferOption) error {
	if opt == nil {
		opt = &baidupcs.TransferOption{}
	}
	link, featureStr, extraCode, err := parseShareTransferArgs(params)
	if err != nil {
		return err
	}

	pcs := GetBaiduPCS()
	tokens, err := pcs.AccessSharePage(featureStr, true)
	if err != nil {
		return fmt.Errorf("%s失败: %w", baidupcs.OperationShareFileSavetoLocal, err)
	}

	if extraCode != "" {
		verifyURL := pcs.GenerateShareQueryURL("verify", map[string]string{
			"shareid":    tokens.ShareID,
			"time":       strconv.FormatInt(time.Now().UnixMilli(), 10),
			"clienttype": "1",
			"uk":         tokens.ShareUK,
		}).String()
		if _, err = pcs.PostShareVerify(verifyURL, link, map[string]string{
			"pwd":       extraCode,
			"vcode":     "null",
			"vcode_str": "null",
			"bdstoken":  tokens.BDSToken,
		}); err != nil {
			return fmt.Errorf("%s失败: %w", baidupcs.OperationShareFileSavetoLocal, err)
		}

		pcs.UpdatePCSCookies(true)

		tokens, err = pcs.AccessSharePage(featureStr, false)
		if err != nil {
			return fmt.Errorf("%s失败: %w", baidupcs.OperationShareFileSavetoLocal, err)
		}
	} else {
		pcs.UpdatePCSCookies(true)
		if tokens.ShareID == "" || tokens.ShareUK == "" {
			tokens, err = pcs.AccessSharePage(featureStr, false)
			if err != nil {
				return fmt.Errorf("%s失败: %w", baidupcs.OperationShareFileSavetoLocal, err)
			}
		}
	}

	queryShareInfoURL := pcs.GenerateShareQueryURL("list", map[string]string{
		"bdstoken": tokens.BDSToken,
		"root":     "1",
		"web":      "5",
		"app_id":   baidupcs.PanAppID,
		"shorturl": featureStr[1:],
		"channel":  "chunlei",
	}).String()
	meta, err := pcs.ExtractShareInfo(queryShareInfoURL, tokens.ShareID, tokens.ShareUK, tokens.BDSToken)
	if err != nil {
		return fmt.Errorf("%s失败: %w", baidupcs.OperationShareFileSavetoLocal, err)
	}

	if opt.FsID != "" {
		meta.FsIDListJSON = "[" + opt.FsID + "]"
		meta.Filename = "user_specified_file"
		meta.ItemNum = 1
	}

	targetPath := GetActiveUser().Workdir
	if targetPath == "" {
		targetPath = "/"
	}
	if meta.ItemNum != 1 && opt.Collect {
		meta.Filename += "等文件"
		targetPath = path.Join(targetPath, meta.Filename)
		_ = pcs.Mkdir(targetPath)
	}

	pcs.UpdatePCSCookies(true)
	result, err := pcs.ExecuteShareTransfer(baidupcs.ShareTransferRequest{
		TransferURL:  meta.TransferURL,
		Referer:      "https://pan.baidu.com/s/" + featureStr,
		FsIDListJSON: meta.FsIDListJSON,
		TargetPath:   targetPath,
	})
	if err != nil {
		return fmt.Errorf("%s失败: %w", baidupcs.OperationShareFileSavetoLocal, err)
	}

	savedName := result.Filename
	if opt.Collect && meta.ItemNum != 1 {
		savedName = meta.Filename
	}
	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "%s成功: 已保存 %s 至 %s", baidupcs.OperationShareFileSavetoLocal, savedName, targetPath)
	}
	if opt.Download {
		if !ui.IsQuiet() && shareTransferDownloadDelay > 0 {
			ui.PrintInfo(os.Stdout, "10s 后开始下载转存文件...")
		}
		if shareTransferDownloadDelay > 0 {
			time.Sleep(shareTransferDownloadDelay)
		}
		downloadPaths := buildTransferDownloadPaths(targetPath, result.Filenames)
		return RunDownload(downloadPaths, &DownloadOptions{DisableWildcard: true})
	}
	return nil
}

func buildTransferDownloadPaths(targetPath string, filenames []string) []string {
	if targetPath == "" {
		targetPath = "/"
	}
	out := make([]string, 0, len(filenames))
	for _, name := range filenames {
		out = append(out, path.Join(targetPath, name))
	}
	return out
}
