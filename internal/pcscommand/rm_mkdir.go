package pcscommand

import (
	"fmt"
	"os"
	"strconv"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
)

// RunRemove 执行 批量删除文件/目录
func RunRemove(paths ...string) error {
	paths, err := matchPathByShellPattern(paths...)
	if err != nil {
		return err
	}

	pnt := func(out *os.File) {
		tb := pcstable.NewTable(out)
		tb.SetHeader([]string{"#", "文件/目录"})
		for k := range paths {
			tb.Append([]string{strconv.Itoa(k), paths[k]})
		}
		tb.Render()
	}

	err = GetBaiduPCS().Remove(paths...)
	if err != nil {
		ui.PrintWarn(os.Stderr, "以下文件/目录删除失败:")
		pnt(os.Stderr)
		return err
	}

	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "已移入网盘回收站（共 %d 项，可通过 'recycle list' 还原）", len(paths))
		pnt(os.Stdout)
	}
	return nil
}

// RunMkdir 执行 创建目录
func RunMkdir(path string) error {
	activeUser := GetActiveUser()
	fullPath := activeUser.PathJoin(path)
	err := GetBaiduPCS().Mkdir(fullPath)
	if err != nil {
		return fmt.Errorf("创建目录 %s 失败: %w", path, err)
	}

	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "已创建目录 %s", fullPath)
	}
	return nil
}
