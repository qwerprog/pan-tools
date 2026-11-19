package pcscommand

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

// RunLocateDownload 执行获取直链
func RunLocateDownload(pcspaths []string, opt *LocateDownloadOption) error {
	if opt == nil {
		opt = &LocateDownloadOption{}
	}

	absPaths, err := matchPathByShellPattern(pcspaths...)
	if err != nil {
		return err
	}

	pcs := GetBaiduPCS()

	if opt.FromPan {
		fds, err := pcs.FilesDirectoriesBatchMeta(absPaths...)
		if err != nil {
			return err
		}

		fidList := make([]int64, 0, len(fds))
		for i := range fds {
			fidList = append(fidList, fds[i].FsID)
		}

		list, err := pcs.LocatePanAPIDownload(fidList...)
		if err != nil {
			return err
		}

		tb := pcstable.NewTable(os.Stdout)
		tb.SetHeader([]string{"#", "fs_id", "路径", "链接"})

		var (
			i          int
			fidStrList = converter.SliceInt64ToString(fidList)
		)
		for k := range fidStrList {
			for i = range list {
				if fidStrList[k] == list[i].FsID {
					tb.Append([]string{strconv.Itoa(k), list[i].FsID, fds[k].Path, list[i].Dlink})
					list = append(list[:i], list[i+1:]...)
					break
				}
			}
		}
		tb.Render()
		if !ui.IsQuiet() {
			ui.PrintInfo(os.Stdout, "注意: 以上链接不能直接访问, 需要携带百度帐号 Cookie 下载")
		}
		return nil
	}

	var (
		firstErr     error
		errItems     []string
		successCount int
	)
	for i, pcspath := range absPaths {
		info, err := pcs.LocateDownload(pcspath)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			errItems = append(errItems, fmt.Sprintf("[%d] %s: %s", i, pcspath, err))
			continue
		}

		successCount++
		if !ui.IsQuiet() {
			ui.PrintInfo(os.Stdout, "[%d] %s", i, pcspath)
		}
		tb := pcstable.NewTable(os.Stdout)
		tb.SetHeader([]string{"#", "链接"})
		for k, u := range info.URLStrings(pcsconfig.Config.EnableHTTPS) {
			tb.Append([]string{strconv.Itoa(k), u.String()})
		}
		tb.Render()
		if !ui.IsQuiet() {
			fmt.Println()
		}
	}
	if successCount > 0 && !ui.IsQuiet() {
		ui.PrintInfo(os.Stdout, "提示: 访问下载链接需将下载器的 User-Agent 设置为: %s", pcsconfig.Config.PanUA)
	}
	if len(errItems) == 1 && len(absPaths) == 1 {
		return fmt.Errorf("获取下载直链失败 (%s): %w", absPaths[0], firstErr)
	}
	if len(errItems) > 0 {
		return fmt.Errorf("获取下载直链失败 (%d/%d): %s (%w)", len(errItems), len(absPaths), strings.Join(errItems, "; "), firstErr)
	}
	return nil
}
