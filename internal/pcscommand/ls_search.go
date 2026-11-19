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

type (
	// LsOptions 列目录可选项
	LsOptions struct {
		Total  bool
		Output ui.OutputMode
	}

	// SearchOptions 搜索可选项
	SearchOptions struct {
		Total   bool
		Recurse bool
		Output  ui.OutputMode
	}

	fileEntryJSON struct {
		FsID  int64  `json:"fs_id"`
		AppID int64  `json:"app_id"`
		Path  string `json:"path"`
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		IsDir bool   `json:"is_dir"`
		Ctime int64  `json:"ctime"`
		Mtime int64  `json:"mtime"`
		MD5   string `json:"md5,omitempty"`
	}
)

const (
	opLs int = iota
	opSearch
)

// RunLs 执行列目录
func RunLs(pcspath string, lsOptions *LsOptions, orderOptions *baidupcs.OrderOptions) error {
	if lsOptions == nil {
		lsOptions = &LsOptions{}
	}

	err := matchPathByShellPatternOnce(&pcspath)
	if err != nil {
		return err
	}

	files, err := GetBaiduPCS().FilesDirectoriesList(pcspath, orderOptions)
	if err != nil {
		return err
	}

	if lsOptions.Output == ui.OutputJSON || lsOptions.Output == ui.OutputPlain {
		return renderMachineFileList(lsOptions.Output, opLs, files)
	}

	if !ui.IsQuiet() {
		ui.PrintInfo(os.Stdout, "当前目录: %s", pcspath)
	}
	renderTable(opLs, lsOptions.Total, pcspath, files)
	return nil
}

// RunSearch 执行搜索
func RunSearch(targetPath, keyword string, opt *SearchOptions) error {
	if opt == nil {
		opt = &SearchOptions{}
	}

	err := matchPathByShellPatternOnce(&targetPath)
	if err != nil {
		return err
	}

	files, err := GetBaiduPCS().Search(targetPath, keyword, opt.Recurse)
	if err != nil {
		return err
	}

	if opt.Output == ui.OutputJSON || opt.Output == ui.OutputPlain {
		return renderMachineFileList(opt.Output, opSearch, files)
	}

	renderTable(opSearch, opt.Total, targetPath, files)
	return nil
}

func renderMachineFileList(mode ui.OutputMode, op int, files baidupcs.FileDirectoryList) error {
	if mode == ui.OutputJSON {
		items := make([]fileEntryJSON, 0, len(files))
		for _, f := range files {
			items = append(items, fileEntryJSON{
				FsID:  f.FsID,
				AppID: f.AppID,
				Path:  f.Path,
				Name:  f.Filename,
				Size:  f.Size,
				IsDir: f.Isdir,
				Ctime: f.Ctime,
				Mtime: f.Mtime,
				MD5:   f.MD5,
			})
		}
		return ui.WriteJSON(os.Stdout, items)
	}

	rows := make([][]string, 0, len(files))
	for _, f := range files {
		kind := "file"
		display := f.Filename
		if op == opSearch {
			display = f.Path
		}
		if f.Isdir {
			kind = "dir"
			if op == opLs {
				display += baidupcs.PathSeparator
			}
		}
		rows = append(rows, []string{
			strconv.FormatInt(f.FsID, 10),
			kind,
			strconv.FormatInt(f.Size, 10),
			pcsutil.FormatTime(f.Mtime),
			display,
		})
	}
	return ui.WriteTSV(os.Stdout, rows)
}

func renderTable(op int, isTotal bool, targetPath string, files baidupcs.FileDirectoryList) {
	colorOn := ui.ColorEnabled(os.Stdout)
	asciiOn := ui.ASCIIOnly()
	tb := pcstable.NewTable(os.Stdout)
	showPath := "名称"
	if op == opSearch {
		showPath = "网盘路径"
	}

	fN, dN := files.Count()
	if isTotal {
		tb.SetHeader([]string{"#", "FS_ID", "APP_ID", "大小", "创建时间", "修改时间", "MD5", showPath})
		tb.SetColumnAlignment([]int{pcstable.AlignDefault, pcstable.AlignRight, pcstable.AlignRight, pcstable.AlignRight, pcstable.AlignLeft, pcstable.AlignLeft, pcstable.AlignLeft, pcstable.AlignLeft})
		for k, file := range files {
			rawName := file.Filename
			if op == opSearch {
				rawName = file.Path
			}
			styledName := ui.StyleFileName(os.Stdout, rawName, file.Isdir, colorOn, asciiOn)
			sizeCell := ui.StyleFileSize(os.Stdout, file.Size, converter.ConvertFileSize(file.Size, 2), file.Isdir, colorOn)
			ctimeCell := ui.StyleMuted(os.Stdout, pcsutil.FormatTime(file.Ctime), colorOn)
			mtimeCell := ui.StyleMuted(os.Stdout, pcsutil.FormatTime(file.Mtime), colorOn)

			md5 := file.MD5
			if !file.Isdir && len(file.BlockList) > 1 {
				md5 = "(分片)" + file.MD5
			}
			tb.Append([]string{
				strconv.Itoa(k),
				ui.StyleMuted(os.Stdout, strconv.FormatInt(file.FsID, 10), colorOn),
				ui.StyleMuted(os.Stdout, strconv.FormatInt(file.AppID, 10), colorOn),
				sizeCell,
				ctimeCell,
				mtimeCell,
				ui.StyleMuted(os.Stdout, md5, colorOn),
				styledName,
			})
		}
	} else {
		tb.SetHeader([]string{"#", "大小", "修改时间", showPath})
		tb.SetColumnAlignment([]int{pcstable.AlignDefault, pcstable.AlignRight, pcstable.AlignLeft, pcstable.AlignLeft})
		for k, file := range files {
			rawName := file.Filename
			if op == opSearch {
				rawName = file.Path
			}
			styledName := ui.StyleFileName(os.Stdout, rawName, file.Isdir, colorOn, asciiOn)
			sizeCell := ui.StyleFileSize(os.Stdout, file.Size, converter.ConvertFileSize(file.Size, 2), file.Isdir, colorOn)
			mtimeCell := ui.StyleMuted(os.Stdout, pcsutil.FormatTime(file.Mtime), colorOn)
			tb.Append([]string{strconv.Itoa(k), sizeCell, mtimeCell, styledName})
		}
	}

	if len(files) > 0 {
		tb.Render()
	}

	if ui.IsQuiet() {
		return
	}

	r := ui.NewRenderer(os.Stdout, &colorOn)
	muted := r.NewStyle().Foreground(ui.Palette.Muted)
	accent := r.NewStyle().Foreground(ui.Palette.Accent).Bold(true)
	primary := r.NewStyle().Foreground(ui.Palette.Primary).Bold(true)

	sumSym := "Σ"
	sep := "·"
	if asciiOn {
		sumSym = "SUM"
		sep = "|"
	}

	fmt.Printf("  %s %s 文件 %s %s 目录 %s 总计 %s %s %s %s\n",
		muted.Render(sumSym),
		primary.Render(strconv.FormatInt(fN, 10)),
		muted.Render(sep),
		primary.Render(strconv.FormatInt(dN, 10)),
		muted.Render(sep),
		accent.Render(converter.ConvertFileSize(files.TotalSize(), 2)),
		muted.Render(sep),
		muted.Render("路径"),
		r.NewStyle().Foreground(ui.Palette.Subtitle).Render(targetPath),
	)
}
