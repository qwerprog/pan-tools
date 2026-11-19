// Package pcsupdate 更新包
package pcsupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsliner"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/downloader"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
)

const (
	// DefaultUpdateRepo 默认检查更新的 GitHub 仓库路径（支持通过环境变量 PANT_UPDATE_REPO 覆盖）。
	DefaultUpdateRepo = "qwerprog/pan-tools"
)

type info struct {
	filename    string
	size        int64
	downloadURL string
}

func resolveUpdateRepo() string {
	if repo := strings.TrimSpace(os.Getenv("PANT_UPDATE_REPO")); repo != "" {
		return repo
	}
	return DefaultUpdateRepo
}

// isNewerSemVer 按语义化版本数值比较 candidateTag 是否严格高于 currentVersion（正确处理 v4.0.10 > v4.0.2 及 -dev 预发布后缀）。
func isNewerSemVer(currentVersion, candidateTag string) bool {
	if strings.Contains(strings.ToLower(candidateTag), "beta") || !strings.HasPrefix(candidateTag, "v") {
		return false
	}
	curCore, curPre := parseSemVerParts(currentVersion)
	candCore, candPre := parseSemVerParts(candidateTag)
	if len(candCore) == 0 {
		return false
	}
	if len(curCore) == 0 {
		return true
	}
	maxLen := len(curCore)
	if len(candCore) > maxLen {
		maxLen = len(candCore)
	}
	for i := 0; i < maxLen; i++ {
		var a, b int
		if i < len(curCore) {
			a = curCore[i]
		}
		if i < len(candCore) {
			b = candCore[i]
		}
		if b != a {
			return b > a
		}
	}
	// 同主次修订号时，正式版高于带 -dev/-rc 等预发布后缀的开发版
	return curPre != "" && candPre == ""
}

func parseSemVerParts(ver string) ([]int, string) {
	v := strings.TrimSpace(strings.TrimPrefix(ver, "v"))
	if v == "" {
		return nil, ""
	}
	if plus := strings.IndexByte(v, '+'); plus >= 0 {
		v = v[:plus]
	}
	var pre string
	if dash := strings.IndexByte(v, '-'); dash >= 0 {
		pre = v[dash+1:]
		v = v[:dash]
	}
	segs := strings.Split(v, ".")
	nums := make([]int, 0, len(segs))
	for _, seg := range segs {
		n, err := strconv.Atoi(seg)
		if err != nil {
			return nil, ""
		}
		nums = append(nums, n)
	}
	return nums, pre
}

// CheckUpdate 检测更新
func CheckUpdate(version string, yes bool) {
	if !pcsutil.AccessRDWR(pcsutil.ExecutablePath()) {
		ui.PrintError(os.Stderr, "程序目录不可写, 无法更新", nil, "")
		return
	}
	repo := resolveUpdateRepo()
	if repo == "" {
		if !ui.IsQuiet() {
			ui.PrintInfo(os.Stdout, "未配置默认更新仓库，请设置环境变量 PANT_UPDATE_REPO（格式: owner/repo）后再检查更新")
		}
		return
	}
	if !ui.IsQuiet() {
		ui.PrintInfo(os.Stdout, "正在检测程序更新，请稍候...")
	}
	c := pcsconfig.Config.HTTPClient()
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := c.Req(http.MethodGet, apiURL, nil, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		ui.PrintError(os.Stderr, "获取更新数据错误", err, "")
		return
	}

	releaseInfo := ReleaseInfo{}
	err = pcsutil.UnmarshalData(resp.Body, &releaseInfo)
	if err != nil {
		ui.PrintError(os.Stderr, "更新版本 JSON 数据解析失败", err, "")
		return
	}

	// 没有更新, 或忽略 Beta 版本, 和版本前缀不符的
	if !isNewerSemVer(version, releaseInfo.TagName) {
		if !ui.IsQuiet() {
			ui.PrintSuccess(os.Stdout, "当前已是最新版本 (%s)", version)
		}
		return
	}

	if !ui.IsQuiet() {
		ui.PrintInfo(os.Stdout, "检测到新版本: %s", releaseInfo.TagName)
	}

	line := pcsliner.NewLiner()
	defer line.Close()

	if !yes {
		y, err := line.State.Prompt("是否进行更新 (y/n): ")
		if err != nil {
			ui.PrintError(os.Stderr, "读取确认输入错误", err, "")
			return
		}

		if y != "y" && y != "Y" {
			if !ui.IsQuiet() {
				ui.PrintInfo(os.Stdout, "已取消更新")
			}
			return
		}
	}

	exp := buildReleaseAssetRegexp(releaseInfo.TagName, runtime.GOOS, runtime.GOARCH)

	var targetList []*info
	for _, asset := range releaseInfo.Assets {
		if asset == nil || asset.State != "uploaded" {
			continue
		}

		if exp.MatchString(asset.Name) {
			targetList = append(targetList, &info{
				filename:    asset.Name,
				size:        asset.Size,
				downloadURL: asset.BrowserDownloadURL,
			})
		}
	}

	var target info
	switch len(targetList) {
	case 0:
		ui.PrintWarn(os.Stderr, "未匹配到当前系统的程序更新文件, GOOS: %s, GOARCH: %s", runtime.GOOS, runtime.GOARCH)
		return
	case 1:
		target = *targetList[0]
	default:
		fmt.Println()
		for k := range targetList {
			fmt.Printf("%d: %s\n", k, targetList[k].filename)
		}

		fmt.Println()
		t, err := line.State.Prompt("输入序号以下载更新: ")
		if err != nil {
			ui.PrintError(os.Stderr, "读取输入错误", err, "")
			return
		}

		i, err := strconv.Atoi(t)
		if err != nil {
			ui.PrintError(os.Stderr, "序号解析错误", err, "")
			return
		}

		if i < 0 || i >= len(targetList) {
			ui.PrintError(os.Stderr, "输入错误: 序号不在范围内", nil, "")
			return
		}

		target = *targetList[i]
	}

	if target.size > 0x7fffffff {
		ui.PrintError(os.Stderr, fmt.Sprintf("更新包体积过大: %d", target.size), nil, "")
		return
	}

	if !ui.IsQuiet() {
		ui.PrintInfo(os.Stdout, "准备下载更新包: %s", target.filename)
	}

	// 开始下载
	buf := rio.NewBuffer(make([]byte, int(target.size)))
	der := downloader.NewDownloader(target.downloadURL, buf, &downloader.Config{
		MaxParallel: 20,
		CacheSize:   10000,
	})
	der.SetClient(c)

	der.OnDownloadStatusEvent(func(status transfer.DownloadStatuser, workersCallback func(downloader.RangeWorkerFunc)) {
		if ui.IsQuiet() {
			return
		}
		leftStr := "-"
		if left := status.TimeLeft(); left >= 0 {
			leftStr = left.String()
		}
		arrow := "↓"
		if ui.ASCIIOnly() {
			arrow = "v"
		}
		var ratio float64
		if total := status.TotalSize(); total > 0 {
			ratio = float64(status.Downloaded()) / float64(total)
		}
		fmt.Print(ui.FormatTransferProgress(
			os.Stdout,
			"update",
			arrow,
			ratio,
			converter.ConvertFileSize(status.Downloaded(), 2),
			converter.ConvertFileSize(status.TotalSize(), 2),
			converter.ConvertFileSize(status.SpeedsPerSecond(), 2),
			(status.TimeElapsed() / 1e7 * 1e7).String(),
			leftStr,
			ui.ColorEnabled(os.Stdout),
			ui.ASCIIOnly(),
			ui.IsTTY(os.Stdout),
			target.filename,
		))
	})
	der.OnFinish(func() {
		ui.Statusln()
	})
	der.OnSuccess(func() {
		if !ui.IsQuiet() {
			ui.PrintSuccess(os.Stdout, "更新包下载完毕")
		}
	})

	err = der.Execute()
	if err != nil {
		ui.PrintError(os.Stderr, "下载更新包发生错误", err, "")
		return
	}

	// 读取并解压更新包（支持 .tar.gz 与 .zip）
	execPath := pcsutil.ExecutablePath()
	fileNum, errTimes, err := extractUpdateArchive(execPath, target.filename, buf.Bytes())
	if err != nil {
		ui.PrintError(os.Stderr, "读取更新文件发生错误", err, "")
		return
	}

	if fileNum == 0 || errTimes == fileNum {
		ui.PrintError(os.Stderr, "更新失败", nil, "")
		return
	}

	ui.PrintSuccess(os.Stdout, "更新完毕, 请重启程序")
}

func buildReleaseAssetRegexp(tagName, goos, goarch string) *regexp.Regexp {
	builder := &strings.Builder{}
	builder.WriteString("pant-" + regexp.QuoteMeta(tagName) + "-" + regexp.QuoteMeta(goos) + "-.*?")

	switch goarch {
	case "amd64":
		builder.WriteString("(amd64|x86_64|x64)")
	case "386":
		builder.WriteString("(386|x86)")
	case "arm":
		builder.WriteString("(armv5|armv7|arm)")
	case "arm64":
		builder.WriteString("arm64")
	case "mips":
		builder.WriteString("mips")
	case "mips64":
		builder.WriteString("mips64")
	case "mipsle":
		builder.WriteString("(mipsle|mipsel)")
	case "mips64le":
		builder.WriteString("(mips64le|mips64el)")
	default:
		builder.WriteString(regexp.QuoteMeta(goarch))
	}

	builder.WriteString(`(\.tar\.gz|\.zip)$`)
	return regexp.MustCompile(builder.String())
}

func extractUpdateArchive(execPath, filename string, data []byte) (fileNum, errTimes int, err error) {
	if strings.HasSuffix(strings.ToLower(filename), ".tar.gz") {
		return extractTarGzArchive(execPath, data)
	}
	return extractZipArchive(execPath, data)
}

func applyExtractedEntry(execPath, entryName string, rc io.Reader) error {
	destPath, pathErr := resolveSafeExtractPath(execPath, entryName)
	if pathErr != nil {
		return pathErr
	}
	baseName := filepath.Base(destPath)
	if baseName == "pant" || baseName == "pant.exe" {
		return update(pcsutil.Executable(), rc)
	}
	return update(destPath, rc)
}

func extractZipArchive(execPath string, data []byte) (fileNum, errTimes int, err error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, 0, err
	}

	for _, zipFile := range reader.File {
		if zipFile == nil || zipFile.FileInfo().IsDir() {
			continue
		}
		fileNum++
		err = func(zf *zip.File) error {
			rc, openErr := zf.Open()
			if openErr != nil {
				return openErr
			}
			defer rc.Close()
			return applyExtractedEntry(execPath, zf.Name, rc)
		}(zipFile)
		if err != nil {
			errTimes++
			fmt.Fprintf(os.Stderr, "发生错误, zip 路径: %s, 错误: %s\n", zipFile.Name, err)
		}
	}
	return fileNum, errTimes, nil
}

func extractTarGzArchive(execPath string, data []byte) (fileNum, errTimes int, err error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return fileNum, errTimes, nextErr
		}
		if hdr == nil || (hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA) {
			continue
		}
		fileNum++
		entryName := strings.TrimPrefix(hdr.Name, "./")
		if err := applyExtractedEntry(execPath, entryName, tr); err != nil {
			errTimes++
			fmt.Fprintf(os.Stderr, "发生错误, tar 路径: %s, 错误: %s\n", hdr.Name, err)
		}
	}
	return fileNum, errTimes, nil
}

func resolveSafeExtractPath(baseDir, zipEntryName string) (string, error) {
	rel := zipEntryName
	if idx := strings.Index(rel, "/"); idx >= 0 {
		rel = rel[idx+1:]
	}
	rel = filepath.FromSlash(strings.TrimSpace(rel))
	if rel == "" {
		return "", errors.New("empty zip entry name")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("unsafe absolute zip entry path: %s", zipEntryName)
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(baseAbs, rel))
	if err != nil {
		return "", err
	}
	relCheck, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil || relCheck == "." || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe zip slip path detected: %s", zipEntryName)
	}
	return targetAbs, nil
}
