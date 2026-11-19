package pcscommand

import (
	"fmt"
	"os"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
)

// RunGetMeta 执行 获取文件/目录的元信息
func RunGetMeta(targetPaths ...string) error {
	targetPaths, err := matchPathByShellPattern(targetPaths...)
	if err != nil {
		return err
	}

	for k, targetPath := range targetPaths {
		data, err := GetBaiduPCS().FilesDirectoriesMeta(targetPath)
		if err != nil {
			return err
		}
		if !ui.IsQuiet() {
			ui.PrintInfo(os.Stdout, "[%d] %s", k, targetPath)
		}
		fmt.Print(pcsfunctions.FormatFileDirectory(data))
	}
	return nil
}
