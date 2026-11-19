package pcscommand

import (
	"fmt"
	"os"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
)

// RunChangeDirectory 执行更改工作目录
func RunChangeDirectory(targetPath string, isList bool) error {
	pcs := GetBaiduPCS()
	err := matchPathByShellPatternOnce(&targetPath)
	if err != nil {
		return err
	}

	data, err := pcs.FilesDirectoriesMeta(targetPath)
	if err != nil {
		return err
	}

	if !data.Isdir {
		return fmt.Errorf("%s 不是一个目录 (文件夹)", targetPath)
	}

	GetActiveUser().Workdir = targetPath
	_ = pcsconfig.Config.Save()

	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "当前目录已切换至 %s", targetPath)
	}

	if isList {
		return RunLs(".", nil, baidupcs.DefaultOrderOptions)
	}
	return nil
}
