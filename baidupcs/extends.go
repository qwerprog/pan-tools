package baidupcs

import (
	"errors"
	"path"
	"strings"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
)

const (
	// ShellPatternCharacters 通配符字符串
	ShellPatternCharacters = "*?[]"
)

var (
	// ErrMatchPathByShellPatternNotAbsPath 不是绝对路径
	ErrMatchPathByShellPatternNotAbsPath = errors.New("not absolute path")
)

func (pcs *BaiduPCS) recurseMatchPathByShellPattern(index int, patternSlice []string, ps []string, pcspaths *[]string) pcserror.Error {
	if index == len(patternSlice) {
		*pcspaths = append(*pcspaths, strings.Join(ps, PathSeparator))
		return nil
	}

	if !strings.ContainsAny(patternSlice[index], ShellPatternCharacters) {
		ps[index] = patternSlice[index]
		return pcs.recurseMatchPathByShellPattern(index+1, patternSlice, ps, pcspaths)
	}

	fds, pcsError := pcs.FilesDirectoriesList(strings.Join(ps[:index], PathSeparator), DefaultOrderOptions)
	if pcsError != nil {
		return pcsError
	}

	for k := range fds {
		if matched, _ := path.Match(patternSlice[index], fds[k].Filename); matched {
			ps[index] = fds[k].Filename
			if err := pcs.recurseMatchPathByShellPattern(index+1, patternSlice, ps, pcspaths); err != nil {
				return err
			}
		}
	}
	return nil
}

// MatchPathByShellPattern 通配符匹配文件路径, pattern 为绝对路径
func (pcs *BaiduPCS) MatchPathByShellPattern(pattern string) (pcspaths []string, pcsError pcserror.Error) {
	errInfo := pcserror.NewPCSErrorInfo(OperationMatchPathByShellPattern)
	errInfo.ErrType = pcserror.ErrTypeOthers

	patternSlice := strings.Split(pcsutil.Escape(path.Clean(pattern), []rune{'['}), PathSeparator)
	if patternSlice[0] != "" {
		errInfo.Err = ErrMatchPathByShellPatternNotAbsPath
		return nil, errInfo
	}

	ps := make([]string, len(patternSlice))
	if err := pcs.recurseMatchPathByShellPattern(1, patternSlice, ps, &pcspaths); err != nil {
		return nil, err
	}
	return pcspaths, nil
}
