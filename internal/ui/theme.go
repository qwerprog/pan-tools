// Package ui 提供 pan-tools CLI 与交互式 REPL 的统一终端呈现层组件。
package ui

import (
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

func init() {
	// 现代 UTF-8 终端在 zh_CN.UTF-8 等 CJK Locale 下仍将框线/圆点/省略号 (╭─╰╯│●…) 渲染为 1 列（与 lipgloss/uniseg 一致）；
	// 锁定 go-runewidth 的 EastAsianWidth=false，避免 peterh/liner 将 boxed 提示符 "╰─❯ " 误判为 6 列而导致光标右偏 2 列。
	// 若用户显式声明 RUNEWIDTH_EASTASIAN=1，则由 ASCIIOnly() 自动降级为纯 7-bit ASCII 符号。
	if os.Getenv("RUNEWIDTH_EASTASIAN") != "1" {
		runewidth.EastAsianWidth = false
		runewidth.DefaultCondition.EastAsianWidth = false
	}
}

const (
	// BrandName 项目展示名称
	BrandName = "pan-tools"
	// AppName 默认命令行可执行程序名
	AppName = "pant"
	// BrandTag 徽标附带标识
	BrandTag = "百度网盘终端客户端"
	// Tagline 默认副标题
	Tagline = "高速分片传输 · 秒传校验 · 交互式网盘管理"
)

// SemanticColors 定义 pan-tools One Dark 主题语义色板。
type SemanticColors struct {
	Accent     lipgloss.Color
	VaultFrame lipgloss.Color
	VaultCore  lipgloss.Color
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Success    lipgloss.Color
	BadgeInk   lipgloss.Color
	Warn       lipgloss.Color
	Fail       lipgloss.Color
	Muted      lipgloss.Color
	Hairline   lipgloss.Color
	Subtitle   lipgloss.Color
}

// Palette 是全局使用的 Atom One Dark 语义调色板。
var Palette = SemanticColors{
	Accent:     lipgloss.Color("#56B6C2"),
	VaultFrame: lipgloss.Color("#528BFF"),
	VaultCore:  lipgloss.Color("#E5E9F0"),
	Primary:    lipgloss.Color("#61AFEF"),
	Secondary:  lipgloss.Color("#C678DD"),
	Success:    lipgloss.Color("#98C379"),
	BadgeInk:   lipgloss.Color("#282C34"),
	Warn:       lipgloss.Color("#E5C07B"),
	Fail:       lipgloss.Color("#E06C75"),
	Muted:      lipgloss.Color("#7F848E"),
	Hairline:   lipgloss.Color("#4B5263"),
	Subtitle:   lipgloss.Color("#ABB2BF"),
}

var (
	noColorOverride     atomic.Bool
	asciiOverride       atomic.Bool
	quietOverride       atomic.Bool
	promptStyleOverride atomic.Value
)

// UIOverrides 保存全局 UI 覆盖开关快照，供 REPL 单条命令执行后恢复会话基线。
type UIOverrides struct {
	NoColor     bool
	ASCII       bool
	Quiet       bool
	PromptStyle string
}

// SnapshotOverrides 获取当前全局 UI 开关的快照。
func SnapshotOverrides() UIOverrides {
	return UIOverrides{
		NoColor:     noColorOverride.Load(),
		ASCII:       asciiOverride.Load(),
		Quiet:       quietOverride.Load(),
		PromptStyle: GetPromptStyleOverride(),
	}
}

// RestoreOverrides 恢复先前保存的全局 UI 开关状态。
func RestoreOverrides(snap UIOverrides) {
	SetNoColor(snap.NoColor)
	SetASCIIOnly(snap.ASCII)
	SetQuiet(snap.Quiet)
	SetPromptStyleOverride(snap.PromptStyle)
}

// SetPromptStyleOverride 设置命令行级别的提示符风格覆盖值（传空字符串清除覆盖）。
func SetPromptStyleOverride(style string) {
	if style == "" {
		promptStyleOverride.Store("")
		return
	}
	if parsed, err := ParsePromptStyle(style); err == nil {
		promptStyleOverride.Store(string(parsed))
		return
	}
	promptStyleOverride.Store("")
}

// GetPromptStyleOverride 获取当前命令行级别的提示符风格覆盖值。
func GetPromptStyleOverride() string {
	v, _ := promptStyleOverride.Load().(string)
	return v
}

// SetNoColor 全局禁用或恢复 ANSI 色彩输出。
func SetNoColor(disable bool) {
	noColorOverride.Store(disable)
	if disable {
		lipgloss.SetColorProfile(termenv.Ascii)
	} else {
		lipgloss.SetColorProfile(termenv.EnvColorProfile())
	}
}

// SetASCIIOnly 全局强制使用纯 7-bit ASCII 字符（无 Unicode 框线/块字符）。
func SetASCIIOnly(ascii bool) {
	asciiOverride.Store(ascii)
}

// SetQuiet 设置静默模式（隐藏欢迎横幅等非必要装饰信息）。
func SetQuiet(quiet bool) {
	quietOverride.Store(quiet)
}

// IsQuiet 返回是否处于静默模式。
func IsQuiet() bool {
	return quietOverride.Load()
}

// IsTTY 判断目标 Writer 是否直接关联终端设备。
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// ColorEnabled 综合全局开关、环境变量及目标终端特性判断是否启用 ANSI 色彩。
func ColorEnabled(w io.Writer) bool {
	if noColorOverride.Load() || os.Getenv("NO_COLOR") != "" || os.Getenv("CLICOLOR") == "0" {
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	if force := os.Getenv("FORCE_COLOR"); force != "" && force != "0" {
		return true
	}
	if force := os.Getenv("CLICOLOR_FORCE"); force != "" && force != "0" {
		return true
	}
	if _, ok := w.(*os.File); ok {
		return IsTTY(w)
	}
	// *strings.Builder 等缓冲最终多通过 os.Stdout 打印，按标准输出 TTY 能力决定。
	return IsTTY(os.Stdout)
}

// ASCIIOnly 判断是否应当降级为纯 ASCII 符号。
func ASCIIOnly() bool {
	if asciiOverride.Load() || os.Getenv("PANT_ASCII") == "1" || os.Getenv("RUNEWIDTH_EASTASIAN") == "1" {
		return true
	}
	for _, key := range []string{"LC_ALL", "LANG"} {
		v := strings.ToUpper(os.Getenv(key))
		if v == "C" || v == "POSIX" {
			return true
		}
	}
	return false
}

// TerminalWidth 返回当前终端可见列宽，失败时从环境变量 COLUMNS 读取或回落至 80。
func TerminalWidth() int {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		if f != nil && IsTTY(f) {
			if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
				return clampWidth(w)
			}
		}
	}
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if w, err := strconv.Atoi(cols); err == nil && w > 0 {
			return clampWidth(w)
		}
	}
	return 80
}

func clampWidth(w int) int {
	if w < 40 {
		return 40
	}
	if w > 240 {
		return 240
	}
	return w
}

// NewRenderer 创建绑定到指定 Writer 且遵循终端真实色深（TrueColor / 256色 / 16色 / Ascii）的 Lipgloss Renderer。
func NewRenderer(w io.Writer, forceColor *bool) *lipgloss.Renderer {
	if w == nil {
		w = os.Stdout
	}
	colorOn := ColorEnabled(w)
	if forceColor != nil {
		colorOn = *forceColor
	}

	r := lipgloss.NewRenderer(w)
	r.SetColorProfile(resolveColorProfile(colorOn))
	return r
}

func resolveColorProfile(colorOn bool) termenv.Profile {
	if !colorOn {
		return termenv.Ascii
	}

	switch strings.TrimSpace(os.Getenv("FORCE_COLOR")) {
	case "3":
		return termenv.TrueColor
	case "2":
		return termenv.ANSI256
	}

	ct := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))
	if ct == "truecolor" || ct == "24bit" {
		return termenv.TrueColor
	}

	if envProf := termenv.EnvColorProfile(); envProf != termenv.Ascii {
		return envProf
	}

	termEnv := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if strings.Contains(termEnv, "truecolor") || strings.Contains(termEnv, "24bit") {
		return termenv.TrueColor
	}
	if strings.Contains(termEnv, "256color") {
		return termenv.ANSI256
	}
	if strings.TrimSpace(os.Getenv("FORCE_COLOR")) == "1" {
		return termenv.ANSI
	}
	if termEnv != "" && termEnv != "dumb" {
		return termenv.ANSI
	}
	return termenv.TrueColor
}
