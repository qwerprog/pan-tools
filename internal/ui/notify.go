package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// PrintSuccess 输出带成功图标的状态消息。
func PrintSuccess(w io.Writer, format string, a ...interface{}) {
	if w == nil {
		w = os.Stdout
	}
	r := NewRenderer(w, nil)
	icon := "✓"
	if ASCIIOnly() {
		icon = "[OK]"
	}
	badge := r.NewStyle().Foreground(Palette.Success).Bold(true).Render(icon)
	fmt.Fprintf(w, "%s %s\n", badge, fmt.Sprintf(format, a...))
}

// PrintWarn 输出带警告图标的提示消息（默认写入 stderr 以免污染 stdout 管道）。
func PrintWarn(w io.Writer, format string, a ...interface{}) {
	if w == nil {
		w = os.Stderr
	}
	r := NewRenderer(w, nil)
	icon := "!"
	if ASCIIOnly() {
		icon = "[!]"
	}
	badge := r.NewStyle().Foreground(Palette.Warn).Bold(true).Render(icon)
	fmt.Fprintf(w, "%s %s\n", badge, fmt.Sprintf(format, a...))
}

// Statusf 在非静默（!IsQuiet）模式下向 stdout 输出普通状态文本。
func Statusf(format string, a ...interface{}) {
	if IsQuiet() {
		return
	}
	fmt.Printf(format, a...)
}

// Statusln 在非静默（!IsQuiet）模式下向 stdout 输出普通状态行。
func Statusln(a ...interface{}) {
	if IsQuiet() {
		return
	}
	fmt.Println(a...)
}

// PrintInfo 输出带引导点的中性信息。
func PrintInfo(w io.Writer, format string, a ...interface{}) {
	if w == nil {
		w = os.Stdout
	}
	r := NewRenderer(w, nil)
	icon := "•"
	if ASCIIOnly() {
		icon = "*"
	}
	badge := r.NewStyle().Foreground(Palette.Accent).Bold(true).Render(icon)
	fmt.Fprintf(w, "%s %s\n", badge, fmt.Sprintf(format, a...))
}

// PrintError 以 Error / Reason / Hint 三段式结构化格式输出错误详情。
func PrintError(w io.Writer, title string, err error, hint string) {
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprint(w, FormatError(w, title, err, hint, ColorEnabled(w), ASCIIOnly()))
}

// FormatError 生成三段式上下文错误文本，便于测试和复用。
func FormatError(w io.Writer, title string, err error, hint string, color, ascii bool) string {
	r := NewRenderer(w, &color)
	icon := "✖"
	if ascii {
		icon = "x"
	}

	errStyle := r.NewStyle().Foreground(Palette.Fail).Bold(true)
	labelStyle := r.NewStyle().Foreground(Palette.Muted).Bold(true)
	hintStyle := r.NewStyle().Foreground(Palette.Primary)

	var sb strings.Builder
	sb.WriteString(errStyle.Render(icon+" 错误: ") + title + "\n")
	if err != nil {
		sb.WriteString("  " + labelStyle.Render("原因:") + " " + err.Error() + "\n")
		if hint == "" {
			hint = FriendlyErrorHint(err)
		}
	}
	if hint != "" {
		sb.WriteString("  " + labelStyle.Render("建议:") + " " + hintStyle.Render(hint) + "\n")
	}
	return sb.String()
}

// FriendlyErrorHint 根据常见百度网盘错误信息推断可操作的排错建议。
func FriendlyErrorHint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "31045"), strings.Contains(msg, "user not exists"), strings.Contains(msg, "未登录"):
		return "运行 'login' 重新登录或刷新账号 BDUSS / STOKEN。"
	case strings.Contains(msg, "31066"), strings.Contains(msg, "31297"), strings.Contains(msg, "file does not exist"):
		return "运行 'ls' 检查当前网盘路径与文件名拼写。"
	case strings.Contains(msg, "31626"), strings.Contains(msg, "user is not authorized"):
		return "尝试调整 'config set -pan_ua ...' 或切换下载模式 '--mode pcs'。"
	case strings.Contains(msg, "31112"), strings.Contains(msg, "exceed quota"):
		return "网盘空间不足，可运行 'rm' 或 'recycle delete -all' 释放空间。"
	case strings.Contains(msg, "未找到命令"):
		return "运行 'pant help' 查看所有可用命令，或运行 'pant ui' 进入交互界面。"
	default:
		return ""
	}
}
