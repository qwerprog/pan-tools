package pcscommand

import (
	"fmt"
	"os"
	"strconv"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcstable"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

// RunRecycleList 执行列出回收站文件列表
func RunRecycleList(page int) error {
	return RunRecycleListWithOutput(page, ui.OutputPretty)
}

// RunRecycleListWithOutput 列出回收站文件列表并支持多种输出格式。
func RunRecycleListWithOutput(page int, mode ui.OutputMode) error {
	if page < 1 {
		page = 1
	}

	pcs := GetBaiduPCS()
	fdl, err := pcs.RecycleList(page)
	if err != nil {
		return err
	}

	if mode == ui.OutputJSON {
		return ui.WriteJSON(os.Stdout, fdl)
	}
	if mode == ui.OutputPlain {
		rows := make([][]string, 0, len(fdl))
		for _, f := range fdl {
			p := f.Path
			if f.Isdir == 1 {
				p += baidupcs.PathSeparator
			}
			rows = append(rows, []string{
				strconv.FormatInt(f.FsID, 10),
				strconv.FormatInt(f.Size, 10),
				strconv.Itoa(f.LeftTime),
				pcsutil.FormatTime(f.Mtime),
				p,
			})
		}
		return ui.WriteTSV(os.Stdout, rows)
	}

	tb := pcstable.NewTable(os.Stdout)
	tb.SetHeader([]string{"#", "fs_id", "文件大小", "创建日期", "修改日期", "md5(截图请打码)", "剩余时间", "路径"})
	tb.SetColumnAlignment([]int{pcstable.AlignDefault, pcstable.AlignRight, pcstable.AlignRight, pcstable.AlignLeft, pcstable.AlignLeft, pcstable.AlignLeft, pcstable.AlignDefault, pcstable.AlignLeft})
	for k, file := range fdl {
		if file.Isdir == 1 {
			tb.Append([]string{strconv.Itoa(k), strconv.FormatInt(file.FsID, 10), "-", pcsutil.FormatTime(file.Ctime), pcsutil.FormatTime(file.Mtime), file.MD5, strconv.Itoa(file.LeftTime), file.Path + baidupcs.PathSeparator})
			continue
		}
		tb.Append([]string{strconv.Itoa(k), strconv.FormatInt(file.FsID, 10), converter.ConvertFileSize(file.Size, 2), pcsutil.FormatTime(file.Ctime), pcsutil.FormatTime(file.Mtime), file.MD5, strconv.Itoa(file.LeftTime), file.Path})
	}

	tb.Render()
	return nil
}

// RunRecycleRestore 执行还原回收站文件或目录
func RunRecycleRestore(fidStrList ...string) error {
	var (
		fidList = converter.SliceStringToInt64(fidStrList)
		pcs     = GetBaiduPCS()
		ex, err = pcs.RecycleRestore(fidList...)
	)
	if err != nil {
		if len(ex) > 0 {
			ui.PrintWarn(os.Stderr, "以下 %d 项 fs_id 已还原成功:", len(ex))
			for k := range ex {
				fmt.Fprintln(os.Stderr, ex[k].FsID)
			}
		}
		return err
	}

	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "还原成功 (共 %d 项)", len(ex))
	}
	return nil
}

// RunRecycleDelete 执行删除回收站文件或目录
func RunRecycleDelete(fidStrList ...string) error {
	var (
		fidList = converter.SliceStringToInt64(fidStrList)
		pcs     = GetBaiduPCS()
		err     = pcs.RecycleDelete(fidList...)
	)
	if err != nil {
		return err
	}

	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "已删除回收站文件 (共 %d 项)", len(fidList))
	}
	return nil
}

// RunRecycleClear 清空回收站
func RunRecycleClear() error {
	pcs := GetBaiduPCS()
	sussNum, err := pcs.RecycleClear()
	if err != nil {
		return err
	}
	if !ui.IsQuiet() {
		ui.PrintSuccess(os.Stdout, "清空回收站成功 (共 %d 项)", sussNum)
	}
	return nil
}
