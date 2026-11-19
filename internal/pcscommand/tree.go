package pcscommand

import (
	"fmt"
	"os"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

type (
	TreeOptions struct {
		Depth    int
		ShowFsid bool
	}
)

func renderTreeBranch(pcspath string, depth int, ancestorsLast []bool, option *TreeOptions, colorOn, asciiOn bool) error {
	files, err := GetBaiduPCS().FilesDirectoriesList(pcspath, baidupcs.DefaultOrderOptions)
	if err != nil {
		return err
	}

	pipeSeg, blankSeg, midBranch, endBranch := "│   ", "    ", "├── ", "└── "
	if asciiOn {
		pipeSeg, blankSeg, midBranch, endBranch = "|   ", "    ", "|-- ", "`-- "
	}

	var indentBuilder strings.Builder
	for _, wasLast := range ancestorsLast {
		if wasLast {
			indentBuilder.WriteString(blankSeg)
		} else {
			indentBuilder.WriteString(pipeSeg)
		}
	}
	indent := indentBuilder.String()

	validFiles := make([]*baidupcs.FileDirectory, 0, len(files))
	for _, f := range files {
		if f != nil {
			validFiles = append(validFiles, f)
		}
	}

	total := len(validFiles)
	for i, file := range validFiles {
		isLast := i == total-1
		branch := midBranch
		if isLast {
			branch = endBranch
		}

		connector := ui.StyleMuted(os.Stdout, indent+branch, colorOn)
		styledName := ui.StyleFileName(os.Stdout, file.Filename, file.Isdir, colorOn, asciiOn)

		var metaParts []string
		if !file.Isdir {
			metaParts = append(metaParts, ui.StyleFileSize(os.Stdout, file.Size, converter.ConvertFileSize(file.Size, 1), false, colorOn))
		}
		if option.ShowFsid {
			metaParts = append(metaParts, ui.StyleMuted(os.Stdout, fmt.Sprintf("fs_id:%d", file.FsID), colorOn))
		}
		metaSuffix := ""
		if len(metaParts) > 0 {
			metaSuffix = "  " + strings.Join(metaParts, "  ")
		}

		fmt.Printf("%s%s%s\n", connector, styledName, metaSuffix)

		if file.Isdir && (option.Depth < 0 || depth < option.Depth) {
			nextAncestors := append(append([]bool(nil), ancestorsLast...), isLast)
			if subErr := renderTreeBranch(file.Path, depth+1, nextAncestors, option, colorOn, asciiOn); subErr != nil {
				return subErr
			}
		}
	}
	return nil
}

// RunTree 列出语义着色的网盘目录树形图
func RunTree(targetPath string, depth int, option *TreeOptions) error {
	if option == nil {
		option = &TreeOptions{Depth: -1}
	}
	if depth == 0 {
		if err := matchPathByShellPatternOnce(&targetPath); err != nil {
			return err
		}
		colorOn := ui.ColorEnabled(os.Stdout)
		asciiOn := ui.ASCIIOnly()
		fmt.Println(ui.StyleFileName(os.Stdout, targetPath, true, colorOn, asciiOn))
		if option.Depth == 0 {
			return nil
		}
		return renderTreeBranch(targetPath, 1, nil, option, colorOn, asciiOn)
	}
	if option.Depth >= 0 && depth > option.Depth {
		return nil
	}
	return renderTreeBranch(targetPath, depth, make([]bool, depth-1), option, ui.ColorEnabled(os.Stdout), ui.ASCIIOnly())
}
