package pcscommand

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/peterh/liner"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsliner"
)

type loginPrompter struct {
	line     *pcsliner.PCSLiner
	reader   *bufio.Reader
	useLiner bool
	stdinTTY bool
}

func newLoginPrompter(in io.Reader) *loginPrompter {
	if in == nil {
		in = os.Stdin
	}
	stdinTTY := false
	if f, ok := in.(*os.File); ok {
		stdinTTY = ui.IsTTY(f)
	}
	stdoutTTY := ui.IsTTY(os.Stdout)
	p := &loginPrompter{
		stdinTTY: stdinTTY,
	}
	if in == os.Stdin && stdinTTY && stdoutTTY && ui.TerminalWidth() >= 30 {
		p.line = pcsliner.NewLiner()
		p.useLiner = true
	} else {
		p.reader = bufio.NewReaderSize(in, 64*1024)
	}
	return p
}

func (p *loginPrompter) Close() {
	if p != nil && p.line != nil {
		_ = p.line.Close()
		p.line = nil
	}
}

func (p *loginPrompter) ensureReader() *bufio.Reader {
	if p.reader == nil {
		p.reader = bufio.NewReaderSize(os.Stdin, 64*1024)
	}
	return p.reader
}

func (p *loginPrompter) readBufferedLine() (string, error) {
	raw, readErr := p.ensureReader().ReadString('\n')
	if readErr != nil && len(raw) == 0 {
		if errors.Is(readErr, io.EOF) {
			return "", ErrLoginCanceled
		}
		return "", readErr
	}
	trimmed := strings.TrimRight(raw, "\r\n")
	if strings.ContainsRune(trimmed, 0x03) || (trimmed == "\x04" && len(raw) == 1) {
		return "", ErrLoginCanceled
	}
	return trimmed, nil
}

func (p *loginPrompter) PromptLine(label string) (string, error) {
	if p.useLiner && p.line != nil {
		val, err := p.line.State.Prompt(label)
		if err == nil {
			return val, nil
		}
		if errors.Is(err, liner.ErrPromptAborted) || errors.Is(err, io.EOF) {
			return "", ErrLoginCanceled
		}
		if !errors.Is(err, liner.ErrNotTerminalOutput) {
			return "", err
		}
		p.useLiner = false
	}
	if p.stdinTTY {
		fmt.Fprint(os.Stderr, label)
	}
	return p.readBufferedLine()
}

// RunCookieLoginPrompt 交互式引导用户粘贴百度网盘 Cookies 字符串。
func RunCookieLoginPrompt() (cookies string, err error) {
	ui.PrintInfo(os.Stderr, "请在浏览器打开 https://pan.baidu.com 登录，按 F12 -> Network 刷新后复制请求头中的 Cookie 字段")
	return runCookieLoginPromptFrom(os.Stdin)
}

func runCookieLoginPromptFrom(in io.Reader) (cookies string, err error) {
	prompter := newLoginPrompter(in)
	defer prompter.Close()

	raw, err := prompter.PromptLine("Cookies > ")
	if err != nil {
		return "", err
	}
	cleaned := pcsconfig.NormalizeCookieHeader(raw)
	if cleaned == "" {
		return "", errors.New("Cookies 不能为空")
	}
	return cleaned, nil
}

// RunBDUSSLoginPrompt 交互式引导用户输入 BDUSS 与网盘 STOKEN。
func RunBDUSSLoginPrompt() (bduss, stoken string, err error) {
	ui.PrintInfo(os.Stderr, "请依次输入百度 BDUSS 与百度网盘 STOKEN（欲使用分享转存等功能建议填写 STOKEN）")
	return runBDUSSLoginPromptFrom(os.Stdin)
}

func runBDUSSLoginPromptFrom(in io.Reader) (bduss, stoken string, err error) {
	prompter := newLoginPrompter(in)
	defer prompter.Close()

	rawBDUSS, err := prompter.PromptLine("BDUSS > ")
	if err != nil {
		return "", "", err
	}
	bduss = strings.TrimSpace(rawBDUSS)
	if bduss == "" {
		return "", "", errors.New("BDUSS 不能为空")
	}

	rawSTOKEN, err := prompter.PromptLine("STOKEN (可选，直接回车跳过) > ")
	if err != nil {
		return "", "", err
	}
	stoken = strings.TrimSpace(rawSTOKEN)
	return bduss, stoken, nil
}
