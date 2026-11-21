// Package tui 提供 pan-tools 全屏交互式网盘文件管理器 (pant ui / browse)。
package tui

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcscommand"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
)

// CloudBackend 抽象网盘核心操作，便于 TUI 状态机与单元测试解耦。
type CloudBackend interface {
	ListDir(dir string, opt *baidupcs.OrderOptions, forceRefresh bool) (baidupcs.FileDirectoryList, error)
	Mkdir(fullPath string) error
	Rename(from, to string) error
	Remove(paths ...string) error
	Quota() (total int64, used int64, err error)
	Membership() (*baidupcs.MembershipInfo, error)
	Download(paths []string) error
	Upload(localPaths []string, saveDir string) error
	SaveWorkdir(dir string)
	UserSummary() (userName string, uid uint64, saveDir string, loggedIn bool)
}

type pcsBackend struct{}

// NewPCSBackend 创建基于当前活动百度账号配置的真实网盘后端。
func NewPCSBackend() CloudBackend {
	return &pcsBackend{}
}

func (b *pcsBackend) pcs() *baidupcs.BaiduPCS {
	return pcsconfig.Config.ActiveUserBaiduPCS()
}

func (b *pcsBackend) ListDir(dir string, opt *baidupcs.OrderOptions, forceRefresh bool) (baidupcs.FileDirectoryList, error) {
	if forceRefresh {
		return b.pcs().FilesDirectoriesList(dir, opt)
	}
	return b.pcs().CacheFilesDirectoriesList(dir, opt)
}

func (b *pcsBackend) Mkdir(fullPath string) error {
	return b.pcs().Mkdir(fullPath)
}

func (b *pcsBackend) Rename(from, to string) error {
	return b.pcs().Rename(from, to)
}

func (b *pcsBackend) Remove(paths ...string) error {
	return b.pcs().Remove(paths...)
}

func (b *pcsBackend) Quota() (int64, int64, error) {
	return b.pcs().QuotaInfo()
}

func (b *pcsBackend) Membership() (*baidupcs.MembershipInfo, error) {
	return b.pcs().MembershipInfo()
}

func (b *pcsBackend) Download(paths []string) error {
	return pcscommand.RunDownload(paths, &pcscommand.DownloadOptions{
		DisableWildcard: true,
	})
}

func (b *pcsBackend) Upload(localPaths []string, saveDir string) error {
	return pcscommand.RunUpload(localPaths, saveDir, &pcscommand.UploadOptions{
		DisableWildcard: true,
	})
}

func (b *pcsBackend) SaveWorkdir(dir string) {
	if u := pcsconfig.Config.ActiveUser(); u != nil && u.Name != "" {
		u.Workdir = dir
		_ = pcsconfig.Config.Save()
	}
}

func (b *pcsBackend) UserSummary() (string, uint64, string, bool) {
	u := pcsconfig.Config.ActiveUser()
	if u == nil || u.Name == "" {
		return "", 0, pcsconfig.Config.SaveDir, false
	}
	return u.Name, u.UID, pcsconfig.Config.SaveDir, true
}

// inProcessTask 实现 tea.ExecCommand，在临时挂起全屏 AltScreen 期间执行高速上传/下载，完成后无缝回到 TUI。
type inProcessTask struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	fn     func() error
}

func (t *inProcessTask) SetStdin(r io.Reader)  { t.stdin = r }
func (t *inProcessTask) SetStdout(w io.Writer) { t.stdout = w }
func (t *inProcessTask) SetStderr(w io.Writer) { t.stderr = w }

func (t *inProcessTask) Run() error {
	err := t.fn()
	if err != nil {
		errOut := t.stderr
		if errOut == nil {
			errOut = os.Stderr
		}
		out := t.stdout
		if out == nil {
			out = os.Stdout
		}
		ui.PrintError(errOut, "传输任务出现错误", err, "")
		fmt.Fprint(out, "\n按 Enter 键返回网盘浏览器...")
		var buf [1]byte
		if t.stdin != nil {
			_, _ = t.stdin.Read(buf[:])
		}
	}
	return err
}

// Run 启动全屏交互式网盘管理器，退出时自动同步最后浏览的工作目录。
func Run(startDir string) error {
	if !ui.IsTTY(os.Stdout) || !ui.IsTTY(os.Stdin) {
		return fmt.Errorf("'pant ui' 需要交互式终端 (TTY)，管道或脚本请使用 'pant ls --json'")
	}
	backend := NewPCSBackend()
	_, _, _, loggedIn := backend.UserSummary()
	if !loggedIn {
		return fmt.Errorf("当前尚未登录百度网盘，请先执行 'pant login'")
	}

	if startDir == "" {
		if u := pcsconfig.Config.ActiveUser(); u != nil && u.Workdir != "" {
			startDir = u.Workdir
		} else {
			startDir = "/"
		}
	} else if u := pcsconfig.Config.ActiveUser(); u != nil {
		startDir = u.PathJoin(startDir)
	}

	m := NewModel(backend, startDir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := finalModel.(*Model); ok && fm.cwd != "" {
		backend.SaveWorkdir(fm.cwd)
	}
	return nil
}
