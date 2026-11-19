package cliapp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/peterh/liner"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsliner"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsliner/args"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/urfave/cli"
)

const (
	metaFetchTimeout = 3000 * time.Millisecond
)

var (
	remoteCompleteCommands = []string{
		"browse", "cd", "cp", "download", "locate", "ls", "match", "meta", "mkdir", "mv", "rename", "rm", "tree", "tui", "ui",
	}
	localCompleteCommands = []string{
		"sumfile", "run",
	}
	// promptStyleReconfigured 标记单条 REPL 命令中是否通过 config set / config reset 显式修改了 prompt_style。
	promptStyleReconfigured atomic.Bool
)

type sessionMetaCache struct {
	uid       uint64
	total     int64
	used      int64
	hasQuota  bool
	vipLabel  string
	isVIP     bool
	isSVIP    bool
	hasVIP    bool
	attempted bool
}

type sessionMetaStore struct {
	mu   sync.RWMutex
	data sessionMetaCache
}

func (s *sessionMetaStore) snapshotFor(uid uint64) sessionMetaCache {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.uid != uid {
		return sessionMetaCache{uid: uid}
	}
	return s.data
}

func (s *sessionMetaStore) reset(uid uint64) {
	s.mu.Lock()
	s.data = sessionMetaCache{uid: uid}
	s.mu.Unlock()
}

func runREPL(c *cli.Context) error {
	app := c.App
	if c.NArg() != 0 {
		return fmt.Errorf("未找到命令: %s", c.Args().Get(0))
	}

	pcsverbose.Verbosef("VERBOSE: pan-tools REPL initialized\n\n")

	var (
		historyFilePath = filepath.Join(pcsconfig.GetConfigDir(), "pcs_command_history.txt")
		line            = pcsliner.NewLiner()
		err             error
	)

	line.History, err = pcsliner.NewLineHistory(historyFilePath)
	if err != nil {
		ui.PrintWarn(os.Stderr, "读取历史命令文件错误: %s", err)
	}

	line.ReadHistory()
	defer func() {
		line.DoWriteHistory()
		line.Close()
	}()

	// tab 自动补全命令与路径 (区分本地路径与网盘路径)
	line.State.SetCompleter(newCompleter(app))

	metaStore := &sessionMetaStore{}
	metaStore.refresh(pcsconfig.Config.ActiveUser(), metaFetchTimeout)
	if !ui.IsQuiet() {
		printWelcomeBanner(app, metaStore)
	}

	baseUIOverrides := ui.SnapshotOverrides()
	baseVerbose := pcsverbose.IsVerbose

	// 在 REPL 会话内拦截无子命令的顶层调用（如仅输入 --prompt-style boxed 或 --），防止递归重入嵌套 REPL。
	origAction := app.Action
	app.Action = func(subCtx *cli.Context) error {
		if subCtx.NArg() != 0 {
			return fmt.Errorf("未找到命令: %s", subCtx.Args().Get(0))
		}
		if subCtx.GlobalIsSet("prompt-style") {
			return fmt.Errorf("未指定子命令（当前已在交互式会话中，持久化切换提示符风格请运行: config set -prompt_style <starship|boxed>）")
		}
		return fmt.Errorf("未指定子命令（输入 'help' 查看可用命令）")
	}
	defer func() {
		app.Action = origAction
	}()

	for {
		activeUser := pcsconfig.Config.ActiveUser()
		if activeUser == nil || activeUser.Name == "" {
			metaStore.reset(0)
		}

		var snap sessionMetaCache
		if activeUser != nil && activeUser.Name != "" {
			snap = metaStore.snapshotFor(activeUser.UID)
		}
		headerLine, inputPrompt := formatPrompt(app, activeUser, snap)
		if headerLine != "" {
			fmt.Fprintln(os.Stdout, headerLine)
		}

		commandLine, err := line.State.Prompt(inputPrompt)
		switch {
		case errors.Is(err, liner.ErrPromptAborted), errors.Is(err, io.EOF):
			return nil
		case err == nil:
			// continue
		default:
			ui.PrintError(os.Stderr, "读取终端输入失败", err, "")
			return nil
		}

		line.State.AppendHistory(commandLine)

		cmdArgs := args.Parse(commandLine)
		if len(cmdArgs) == 0 {
			continue
		}
		if cmdArgs[0] == "exit" || cmdArgs[0] == "quit" {
			return nil
		}

		s := []string{os.Args[0]}
		s = append(s, cmdArgs...)

		// 恢复原始终端状态，防止运行子命令时终端处于 raw mode
		line.Pause()
		promptStyleReconfigured.Store(false)
		preScanGlobalFlags(s)
		subErr := c.App.Run(s)
		line.Resume()
		if subErr != nil {
			var exitCoder cli.ExitCoder
			if errors.As(subErr, &exitCoder) && exitCoder.ExitCode() == 0 && resolveREPLCommandName(app, cmdArgs) == "quit" {
				return nil
			}
			if !errors.As(subErr, &exitCoder) || exitCoder.ExitCode() != 0 {
				ui.PrintError(os.Stderr, "命令执行失败", subErr, "")
			}
		}
		// 若用户在交互式会话中显式执行了 config set -prompt_style / config reset，立即切换当前会话提示符风格（覆盖启动时的命令行参数或环境变量）。
		if subErr == nil && promptStyleReconfigured.Load() {
			baseUIOverrides.PromptStyle = string(ui.NormalizePromptStyle(pcsconfig.Config.PromptStyle))
		}
		// 恢复 REPL 会话级 UI 基线，避免单次命令中的 -q/--no-color/--ascii 污染后续交互提示符与输出
		ui.RestoreOverrides(baseUIOverrides)
		pcsverbose.IsVerbose = baseVerbose

		// 登录后切换账号或执行会改变配额的命令时刷新提示符会员/配额缓存，且在离线/过期时不再每次同步阻塞 3 秒。
		if cu := pcsconfig.Config.ActiveUser(); cu != nil && cu.Name != "" {
			cur := metaStore.snapshotFor(cu.UID)
			cmdName := resolveREPLCommandName(app, cmdArgs)
			switch {
			case cmdName == "login" || cmdName == "su" || cmdName == "quota" || !cur.attempted:
				metaStore.refresh(cu, metaFetchTimeout)
			case isQuotaMutatingCommand(cmdName) && subErr == nil:
				if cur.hasQuota || cur.hasVIP {
					metaStore.refresh(cu, metaFetchTimeout)
				} else {
					go metaStore.refresh(cu, metaFetchTimeout)
				}
			}
		} else {
			metaStore.reset(0)
		}
		fmt.Println()
	}
}

func resolveREPLCommandName(app *cli.App, cmdArgs []string) string {
	for i := 0; i < len(cmdArgs); i++ {
		tok := cmdArgs[i]
		if tok == "--" {
			if i+1 < len(cmdArgs) {
				tok = cmdArgs[i+1]
				if app != nil {
					if cmd := app.Command(tok); cmd != nil {
						return cmd.FullName()
					}
				}
				return tok
			}
			return ""
		}
		if strings.HasPrefix(tok, "-") && tok != "-" {
			trimmed := strings.TrimLeft(tok, "-")
			if (trimmed == "prompt-style" || trimmed == "prompt_style") && i+1 < len(cmdArgs) {
				i++
			}
			continue
		}
		if app != nil {
			if cmd := app.Command(tok); cmd != nil {
				return cmd.FullName()
			}
		}
		return tok
	}
	return ""
}

func isQuotaMutatingCommand(cmdName string) bool {
	switch cmdName {
	case "upload", "rm", "cp", "transfer", "recycle", "offlinedl", "ui":
		return true
	default:
		return false
	}
}

func (s *sessionMetaStore) refresh(activeUser *pcsconfig.Baidu, timeout time.Duration) {
	if activeUser == nil || activeUser.Name == "" {
		return
	}
	uid := activeUser.UID
	s.mu.Lock()
	if s.data.uid != uid {
		s.data = sessionMetaCache{uid: uid}
	}
	s.data.attempted = true
	s.mu.Unlock()

	if !ui.IsTTY(os.Stdout) {
		return
	}
	quotaPCS := activeUser.BaiduPCS()
	vipPCS := activeUser.BaiduPCS()
	if quotaPCS == nil || vipPCS == nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		q, u, err := quotaPCS.QuotaInfo()
		if err != nil || q <= 0 {
			return
		}
		s.mu.Lock()
		if s.data.uid == uid {
			s.data.total = q
			s.data.used = u
			s.data.hasQuota = true
		}
		s.mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		mi, err := vipPCS.MembershipInfo()
		if err != nil || mi == nil {
			return
		}
		s.mu.Lock()
		if s.data.uid == uid {
			s.data.vipLabel = mi.Label
			s.data.isVIP = mi.IsVIP
			s.data.isSVIP = mi.IsSVIP
			s.data.hasVIP = true
		}
		s.mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func printWelcomeBanner(app *cli.App, metaStore *sessionMetaStore) {
	activeUser := pcsconfig.Config.ActiveUser()
	var meta sessionMetaCache
	if activeUser != nil && activeUser.Name != "" {
		meta = metaStore.snapshotFor(activeUser.UID)
	}
	info := ui.BannerInfo{
		AppName:    app.Name,
		BrandName:  ui.BrandName,
		Version:    app.Version,
		Platform:   runtime.GOOS + "/" + runtime.GOARCH,
		QuotaTotal: meta.total,
		QuotaUsed:  meta.used,
		HasQuota:   meta.hasQuota,
	}
	if activeUser != nil && activeUser.Name != "" {
		info.LoggedIn = true
		info.UserName = activeUser.Name
		info.UID = activeUser.UID
		info.Workdir = activeUser.Workdir
		if meta.hasVIP {
			info.VIPLabel = meta.vipLabel
			info.IsVIP = meta.isVIP
			info.IsSVIP = meta.isSVIP
		}
	}

	fmt.Println()
	fmt.Println(ui.RenderBanner(os.Stdout, info))
	fmt.Println()
}

func formatPrompt(app *cli.App, activeUser *pcsconfig.Baidu, meta sessionMetaCache) (string, string) {
	appName := ui.AppName
	if app != nil && app.Name != "" {
		appName = app.Name
	}
	ctx := ui.PromptContext{
		Brand:     ui.BrandName,
		AppName:   appName,
		Style:     ui.EffectivePromptStyle(pcsconfig.Config.PromptStyle),
		TermWidth: ui.TerminalWidth(),
	}
	if activeUser != nil && activeUser.Name != "" {
		ctx.LoggedIn = true
		ctx.UserName = activeUser.Name
		ctx.Workdir = activeUser.Workdir
		if meta.hasVIP {
			ctx.VIPLabel = meta.vipLabel
			ctx.IsVIP = meta.isVIP
			ctx.IsSVIP = meta.isSVIP
		}
		if meta.hasQuota && meta.total > 0 {
			ctx.HasQuota = true
			ctx.QuotaUsed = meta.used
			ctx.QuotaTotal = meta.total
			ctx.QuotaRatio = float64(meta.used) / float64(meta.total)
		}
	}
	return ui.FormatPrompt(os.Stdout, ctx)
}

func newCompleter(app *cli.App) func(line string) []string {
	pcsRuneFunc := func(r rune) bool {
		switch r {
		case '\'', '"':
			return true
		}
		return unicode.IsSpace(r)
	}

	return func(line string) (s []string) {
		var (
			lineArgs = args.Parse(line)
			numArgs  = len(lineArgs)
			closed   = strings.LastIndex(line, " ") == len(line)-1
		)

		for _, cmd := range app.Commands {
			for _, name := range cmd.Names() {
				if !strings.HasPrefix(name, line) {
					continue
				}
				s = append(s, name+" ")
			}
		}

		switch numArgs {
		case 0:
			return
		case 1:
			if !closed {
				return
			}
		}

		thisCmd := app.Command(lineArgs[0])
		if thisCmd == nil {
			return
		}

		// 子命令补全 (例如 config / share / offlinedl / recycle / tool)
		if len(thisCmd.Subcommands) > 0 && ((numArgs == 1 && closed) || (numArgs == 2 && !closed)) {
			prefix := ""
			if numArgs == 2 && !closed {
				prefix = lineArgs[1]
			}
			for _, sub := range thisCmd.Subcommands {
				for _, name := range sub.Names() {
					if strings.HasPrefix(name, prefix) {
						s = append(s, lineArgs[0]+" "+name+" ")
					}
				}
			}
			return
		}

		cmdName := thisCmd.FullName()
		isLocal := pcsutil.ContainsString(localCompleteCommands, cmdName)
		isRemote := pcsutil.ContainsString(remoteCompleteCommands, cmdName)

		// upload 命令: 除最后一个目标网盘目录外优先补全本地文件; 当参数达到2个以上时同时候选网盘目录
		completeBoth := false
		if cmdName == "upload" {
			isLocal = true
			if (closed && numArgs >= 2) || (!closed && numArgs >= 3) {
				completeBoth = true
			}
		} else if cmdName == "share" && numArgs >= 2 {
			switch lineArgs[1] {
			case "set", "s":
				isRemote = true
			}
		} else if cmdName == "tool" && numArgs >= 2 {
			switch lineArgs[1] {
			case "sumfile", "sf", "enc", "dec":
				isLocal = true
			case "match":
				isRemote = true
			}
		}

		if !isLocal && !isRemote {
			return
		}

		var (
			runeFunc   = unicode.IsSpace
			targetPath string
			prefixArgs []string
		)

		if !closed {
			targetPath = lineArgs[numArgs-1]
			prefixArgs = append([]string(nil), lineArgs[:numArgs-1]...)
		} else {
			prefixArgs = append([]string(nil), lineArgs...)
		}
		pcsutil.EscapeStringsByRuneFunc(prefixArgs, runeFunc)

		switch {
		case targetPath == "." || strings.HasSuffix(targetPath, "/."):
			s = append(s, line+"/")
			return
		case targetPath == ".." || strings.HasSuffix(targetPath, "/.."):
			s = append(s, line+"/")
			return
		}

		if isLocal {
			s = append(s, completeLocalPaths(prefixArgs, targetPath, closed, pcsRuneFunc)...)
		}
		if isRemote || completeBoth {
			s = append(s, completeRemotePaths(prefixArgs, targetPath, closed, pcsRuneFunc)...)
		}
		return
	}
}

func completeLocalPaths(prefixArgs []string, targetPath string, closed bool, escapeFunc func(r rune) bool) (s []string) {
	var (
		dir  = "."
		base = ""
	)
	if !closed && targetPath != "" {
		if strings.HasSuffix(targetPath, "/") || strings.HasSuffix(targetPath, string(os.PathSeparator)) {
			dir = targetPath
		} else {
			dir = filepath.Dir(targetPath)
			base = filepath.Base(targetPath)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".") && strings.HasPrefix(base, ".") {
			continue
		}
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if base != "" && !strings.HasPrefix(name, base) {
			continue
		}

		var candidate string
		if dir == "." && !strings.HasPrefix(targetPath, "./") {
			candidate = name
		} else {
			candidate = filepath.Join(dir, name)
		}
		escaped := pcsutil.EscapeByRuneFunc(candidate, escapeFunc)
		appendLine := strings.Join(append(append([]string(nil), prefixArgs...), escaped), " ")
		if entry.IsDir() {
			s = append(s, appendLine+"/")
		} else {
			s = append(s, appendLine+" ")
		}
	}
	return s
}

func completeRemotePaths(prefixArgs []string, targetPath string, closed bool, escapeFunc func(r rune) bool) (s []string) {
	var (
		activeUser = pcsconfig.Config.ActiveUser()
		pcs        = pcsconfig.Config.ActiveUserBaiduPCS()
		slashPath  = filepath.ToSlash(targetPath)
		targetDir  string
		isAbs      = strings.HasPrefix(slashPath, "/")
		isDir      = strings.HasSuffix(slashPath, "/")
	)
	if activeUser == nil || pcs == nil {
		return nil
	}

	if isAbs {
		targetDir = path.Dir(slashPath)
	} else {
		wd := activeUser.Workdir
		if wd == "" {
			wd = "/"
		}
		joined := path.Join(wd, slashPath)
		if !isDir {
			targetDir = path.Dir(joined)
		} else {
			targetDir = joined
		}
	}

	files, err := pcs.CacheFilesDirectoriesList(targetDir, baidupcs.DefaultOrderOptions)
	if err != nil {
		return nil
	}

	for _, file := range files {
		if file == nil {
			continue
		}

		var candidate string
		if !closed {
			cleanJoin := path.Clean(path.Join(targetDir, path.Base(slashPath)))
			if strings.HasPrefix(file.Path, cleanJoin) {
				candidate = path.Clean(path.Join(path.Dir(slashPath), file.Filename))
			} else if path.Base(targetDir) == path.Base(slashPath) {
				candidate = path.Join(slashPath, file.Filename)
			} else {
				continue
			}
		} else {
			candidate = file.Filename
		}

		appendLine := strings.Join(append(append([]string(nil), prefixArgs...), pcsutil.EscapeByRuneFunc(candidate, escapeFunc)), " ")
		if file.Isdir {
			s = append(s, appendLine+"/")
		} else {
			s = append(s, appendLine+" ")
		}
	}
	return s
}
