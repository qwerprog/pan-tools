package pcscommand

import (
	"fmt"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsfunctions"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"os"
	"path"
	"strings"
)

// RunCopy 执行 批量拷贝文件/目录
func RunCopy(paths ...string) error {
	return runCpMvOp("copy", paths...)
}

// RunMove 执行 批量 重命名/移动 文件/目录
func RunMove(paths ...string) error {
	return runCpMvOp("move", paths...)
}

// RunRename 执行 重命名单个文件/目录 (支持直接传入新文件名或目标路径)
func RunRename(from, to string) error {
	err := matchPathByShellPatternOnce(&from)
	if err != nil {
		return err
	}
	var targetPath string
	if strings.Contains(to, "/") {
		targetPath = GetActiveUser().PathJoin(to)
	} else {
		targetPath = path.Clean(path.Join(path.Dir(from), to))
	}
	err = GetBaiduPCS().Rename(from, targetPath)
	if err != nil {
		return fmt.Errorf("重命名失败 (%s -> %s): %w", from, targetPath, err)
	}
	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "已重命名 %s -> %s", from, targetPath)
	}
	return nil
}

func runCpMvOp(op string, paths ...string) error {
	err := cpmvPathValid(paths...) // 检查路径的有效性, 目前只是判断数量
	if err != nil {
		return fmt.Errorf("%s 路径参数错误: %w", op, err)
	}

	from, to := cpmvParsePath(paths...) // 分割

	from, err = matchPathByShellPattern(from...)
	if err != nil {
		return err
	}
	to = GetActiveUser().PathJoin(to)

	// 尝试匹配
	if strings.ContainsAny(to, baidupcs.ShellPatternCharacters) {
		tos, _ := matchPathByShellPattern(to)

		switch len(tos) {
		case 0:
			// do nothing
		case 1:
			to = tos[0]
		default:
			return fmt.Errorf("目标目录有 %d 条匹配结果, 请检查通配符", len(tos))
		}
	}

	pcs := GetBaiduPCS()
	toInfo, pcsError := pcs.FilesDirectoriesMeta(to)
	switch {
	case toInfo != nil && toInfo.Path != path.Clean(to):
		fallthrough
	case pcsError != nil && pcsError.GetErrType() == pcserror.ErrTypeRemoteError:
		// 判断路径是否存在
		// 如果不存在, 则为重命名或同目录拷贝操作

		// 如果 from 数不是1, 则意义不明确.
		if len(from) != 1 {
			return fmt.Errorf("目标目录 %s 不存在, 无法批量移动或拷贝多个文件", to)
		}

		if op == "copy" { // 拷贝
			err = pcs.Copy(&baidupcs.CpMvJSON{
				From: from[0],
				To:   to,
			})
			if err != nil {
				return fmt.Errorf("文件/目录拷贝失败 (%s <-> %s): %w", from[0], to, err)
			}
			if !ui.IsQuiet() {
				ui.PrintSuccess(os.Stdout, "已拷贝 %s -> %s", from[0], to)
			}
		} else { // 重命名
			err = pcs.Rename(from[0], path.Clean(to))
			if err != nil {
				return fmt.Errorf("重命名失败 (%s -> %s): %w", from[0], to, err)
			}
			if !ui.IsQuiet() {
				ui.PrintSuccess(os.Stdout, "已重命名 %s -> %s", from[0], to)
			}
		}
		return nil
	case pcsError != nil && pcsError.GetErrType() != pcserror.ErrTypeRemoteError:
		return pcsError
	}

	if !toInfo.Isdir {
		return fmt.Errorf("目标 %s 不是一个目录, 操作失败", toInfo.Path)
	}

	cj := new(baidupcs.CpMvListJSON)
	cj.List = make([]*baidupcs.CpMvJSON, len(from))
	for k := range from {
		cj.List[k] = &baidupcs.CpMvJSON{
			From: from[k],
			To:   path.Clean(to + baidupcs.PathSeparator + path.Base(from[k])),
		}
	}

	switch op {
	case "copy":
		err = pcs.Copy(cj.List...)
		if err != nil {
			ui.PrintWarn(os.Stderr, "以下文件/目录拷贝失败:")
			fmt.Fprint(os.Stderr, pcsfunctions.FormatCpMvList(cj))
			return err
		}
		if !ui.IsQuiet() {
			ui.PrintSuccess(os.Stdout, "已拷贝 %d 项文件/目录", len(cj.List))
			fmt.Print(pcsfunctions.FormatCpMvList(cj))
		}
	case "move":
		err = pcs.Move(cj.List...)
		if err != nil {
			ui.PrintWarn(os.Stderr, "以下文件/目录移动失败:")
			fmt.Fprint(os.Stderr, pcsfunctions.FormatCpMvList(cj))
			return err
		}
		if !ui.IsQuiet() {
			ui.PrintSuccess(os.Stdout, "已移动 %d 项文件/目录", len(cj.List))
			fmt.Print(pcsfunctions.FormatCpMvList(cj))
		}
	default:
		return fmt.Errorf("unknown operation: %s", op)
	}
	return nil
}

// cpmvPathValid 检查路径的有效性
func cpmvPathValid(paths ...string) (err error) {
	if len(paths) <= 1 {
		return fmt.Errorf("参数不完整")
	}

	return nil
}

// cpmvParsePath 解析路径
func cpmvParsePath(paths ...string) (from []string, to string) {
	if len(paths) == 0 {
		return nil, ""
	}
	from = paths[:len(paths)-1]
	to = paths[len(paths)-1]
	return
}
