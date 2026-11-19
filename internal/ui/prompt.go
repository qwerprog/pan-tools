package ui

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

const (
	// NameShortDisplayNum 提示符中目录末端保留的最大显示长度。
	NameShortDisplayNum = 24
	// EnvPromptStyle 控制交互式 REPL 提示符风格的环境变量。
	EnvPromptStyle = "PANT_PROMPT_STYLE"
)

// PromptStyle 定义交互式 REPL 提示符的布局风格。
type PromptStyle string

const (
	// PromptStyleStarship 极简双行右对齐提示符（默认，Starship 风格）。
	PromptStyleStarship PromptStyle = "starship"
	// PromptStyleBoxed 圆角框线双行提示符（╭─ pan-tools ▏ ... / ╰─❯ ）。
	PromptStyleBoxed PromptStyle = "boxed"
	// DefaultPromptStyle 默认提示符风格。
	DefaultPromptStyle = PromptStyleStarship
)

// ParsePromptStyle 解析并校验提示符风格名称（大小写不敏感）。
func ParsePromptStyle(raw string) (PromptStyle, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(PromptStyleStarship), "minimal":
		return PromptStyleStarship, nil
	case string(PromptStyleBoxed), "box":
		return PromptStyleBoxed, nil
	default:
		return "", fmt.Errorf("无效的提示符风格 %q (可选值: %s, %s)", raw, PromptStyleStarship, PromptStyleBoxed)
	}
}

// NormalizePromptStyle 将任意配置字符串归一化为合法的 PromptStyle，空值或未知值回退至默认风格。
func NormalizePromptStyle(raw string) PromptStyle {
	if parsed, err := ParsePromptStyle(raw); err == nil {
		return parsed
	}
	return DefaultPromptStyle
}

// EffectivePromptStyle 按「命令行标志 > 环境变量 PANT_PROMPT_STYLE > 配置文件值 > 默认值」优先级解析提示符风格。
func EffectivePromptStyle(configured string) PromptStyle {
	if ov := GetPromptStyleOverride(); ov != "" {
		if parsed, err := ParsePromptStyle(ov); err == nil {
			return parsed
		}
	}
	if env := strings.TrimSpace(os.Getenv(EnvPromptStyle)); env != "" {
		if parsed, err := ParsePromptStyle(env); err == nil {
			return parsed
		}
	}
	return NormalizePromptStyle(configured)
}

// PromptContext 描述交互式 REPL 提示符所需的上下文状态。
type PromptContext struct {
	Brand      string
	AppName    string
	Style      PromptStyle
	UserName   string
	VIPLabel   string
	IsVIP      bool
	IsSVIP     bool
	Workdir    string
	LoggedIn   bool
	QuotaRatio float64
	QuotaUsed  int64
	QuotaTotal int64
	HasQuota   bool
	TermWidth  int
	Color      *bool
	ASCII      *bool
}

// FormatPrompt 根据风格配置生成两行式 REPL 提示符（默认 starship 极简右对齐，可选 boxed 圆角框线）：
//
// starship:
//
//	alice  SVIP  ~/工作/架构设计                        54.2% · 1.1TB/2.0TB
//	❯
//
// boxed:
//
//	╭─ pan-tools ▏ ● alice [VIP: SVIP] [dir: .../架构设计] [quota: 54.2%]
//	╰─❯
//
// 返回的 inputPrompt 仅含可打印字符，绝不包含 ANSI 控制序列，100% 兼容 peterh/liner。
func FormatPrompt(w io.Writer, ctx PromptContext) (headerLine string, inputPrompt string) {
	colorOn := ColorEnabled(w)
	if ctx.Color != nil {
		colorOn = *ctx.Color
	}
	asciiOn := ASCIIOnly()
	if ctx.ASCII != nil {
		asciiOn = *ctx.ASCII
	}

	width := ctx.TermWidth
	if width <= 0 {
		width = TerminalWidth()
	}

	brand := sanitizePromptField(ctx.Brand)
	if brand == "" {
		brand = BrandName
	}
	ctx.UserName = sanitizePromptField(ctx.UserName)
	ctx.VIPLabel = sanitizePromptField(ctx.VIPLabel)
	ctx.Workdir = sanitizePromptField(ctx.Workdir)

	style := EffectivePromptStyle("")
	if ctx.Style != "" {
		style = NormalizePromptStyle(string(ctx.Style))
	}

	if style == PromptStyleBoxed {
		return formatBoxedPrompt(w, ctx, brand, width, colorOn, asciiOn)
	}
	return formatStarshipPrompt(w, ctx, brand, width, colorOn, asciiOn)
}

func formatStarshipPrompt(w io.Writer, ctx PromptContext, brand string, width int, colorOn, asciiOn bool) (string, string) {
	r := NewRenderer(w, &colorOn)
	muted := r.NewStyle().Foreground(Palette.Muted)
	brandStyle := r.NewStyle().Foreground(Palette.Primary).Bold(true)
	userStyle := r.NewStyle().Foreground(Palette.VaultCore).Bold(true)
	dirStyle := r.NewStyle().Foreground(Palette.Accent).Bold(true)
	warnStyle := r.NewStyle().Foreground(Palette.Warn)

	promptArrow := "❯ "
	offlineDot := "○"
	sep := " · "
	ellipsis := "…"
	if asciiOn {
		promptArrow = "> "
		offlineDot = "o"
		sep = " | "
		ellipsis = "..."
	}

	maxW := max(4, width-1)
	if !ctx.LoggedIn || ctx.UserName == "" {
		left := brandStyle.Render(brand) + "  " + warnStyle.Render(offlineDot+" 未登录") + " " + muted.Render("(输入 login 登录)")
		if lipgloss.Width(left) > maxW {
			left = ansi.Truncate(left, maxW, ellipsis)
		}
		return left, promptArrow
	}

	var leftParts []string
	leftParts = append(leftParts, userStyle.Render(ctx.UserName))
	if ctx.VIPLabel != "" {
		kind := BadgeOffline
		if ctx.IsSVIP {
			kind = BadgeWarn
		} else if ctx.IsVIP {
			kind = BadgeInfo
		}
		leftParts = append(leftParts, RenderBadge(w, kind, ctx.VIPLabel, colorOn))
	}
	leftParts = append(leftParts, dirStyle.Render(shortHomeWorkdir(ctx.Workdir, asciiOn)))
	leftLine := strings.Join(leftParts, "  ")

	if !ctx.HasQuota {
		if lipgloss.Width(leftLine) > maxW {
			leftLine = ansi.Truncate(leftLine, maxW, ellipsis)
		}
		return leftLine, promptArrow
	}

	pct := clampRatio(ctx.QuotaRatio) * 100
	qColor := Palette.Subtitle
	if ctx.QuotaRatio >= 0.75 {
		qColor = quotaTone(ctx.QuotaRatio)
	}
	qStyle := r.NewStyle().Foreground(qColor)
	if ctx.QuotaRatio >= 0.85 {
		qStyle = qStyle.Bold(true)
	}

	pctRendered := qStyle.Render(fmt.Sprintf("%.1f%%", pct))
	rightLine := pctRendered
	if ctx.QuotaTotal > 0 {
		usedStr := converter.ConvertFileSize(ctx.QuotaUsed, 1)
		totalStr := converter.ConvertFileSize(ctx.QuotaTotal, 1)
		rightLine = pctRendered + muted.Render(sep+usedStr+"/"+totalStr)
	}

	gap := width - lipgloss.Width(leftLine) - lipgloss.Width(rightLine) - 1
	if gap >= 2 {
		return leftLine + strings.Repeat(" ", gap) + rightLine, promptArrow
	}

	// 窄终端回退：优先保留紧凑百分比并限制整行不超宽折行
	compactGap := width - lipgloss.Width(leftLine) - lipgloss.Width(pctRendered) - 1
	if compactGap >= 2 {
		return leftLine + strings.Repeat(" ", compactGap) + pctRendered, promptArrow
	}
	return ansi.Truncate(leftLine+"  "+pctRendered, maxW, ellipsis), promptArrow
}

func formatBoxedPrompt(w io.Writer, ctx PromptContext, brand string, width int, colorOn, asciiOn bool) (string, string) {
	r := NewRenderer(w, &colorOn)
	frameStyle := r.NewStyle().Foreground(Palette.Hairline)
	muted := r.NewStyle().Foreground(Palette.Muted)
	brandStyle := r.NewStyle().Foreground(Palette.Primary).Bold(true)
	okStyle := r.NewStyle().Foreground(Palette.Success).Bold(true)
	userStyle := r.NewStyle().Foreground(Palette.VaultCore).Bold(true)
	dirStyle := r.NewStyle().Foreground(Palette.Accent).Bold(true)
	warnStyle := r.NewStyle().Foreground(Palette.Warn)

	topPrefix := "╭─ "
	barSep := " ▏ "
	inputPrompt := "╰─❯ "
	onlineDot := "●"
	offlineDot := "○"
	ellipsis := "…"
	if asciiOn {
		topPrefix = "+- "
		barSep = " | "
		inputPrompt = "+-> "
		onlineDot = "*"
		offlineDot = "o"
		ellipsis = "..."
	}

	maxW := max(4, width-1)
	prefix := frameStyle.Render(topPrefix) + brandStyle.Render(brand) + frameStyle.Render(barSep)

	if !ctx.LoggedIn || ctx.UserName == "" {
		line := prefix + warnStyle.Render(offlineDot+" 未登录") + " " + muted.Render("(输入 login 登录)")
		if lipgloss.Width(line) > maxW {
			line = ansi.Truncate(line, maxW, ellipsis)
		}
		return line, inputPrompt
	}

	var parts []string
	parts = append(parts, okStyle.Render(onlineDot)+" "+userStyle.Render(ctx.UserName))

	if ctx.VIPLabel != "" {
		vipColor := Palette.Subtitle
		vipBold := false
		if ctx.IsSVIP {
			vipColor = Palette.Warn
			vipBold = true
		} else if ctx.IsVIP {
			vipColor = Palette.Primary
			vipBold = true
		}
		vipStyle := r.NewStyle().Foreground(vipColor)
		if vipBold {
			vipStyle = vipStyle.Bold(true)
		}
		parts = append(parts, muted.Render("[VIP: ")+vipStyle.Render(ctx.VIPLabel)+muted.Render("]"))
	}

	parts = append(parts, muted.Render("[dir: ")+dirStyle.Render(shortBoxedWorkdir(ctx.Workdir))+muted.Render("]"))

	if ctx.HasQuota {
		pct := clampRatio(ctx.QuotaRatio) * 100
		qStyle := r.NewStyle().Foreground(quotaTone(ctx.QuotaRatio))
		if ctx.QuotaRatio >= 0.85 {
			qStyle = qStyle.Bold(true)
		}
		parts = append(parts, muted.Render("[quota: ")+qStyle.Render(fmt.Sprintf("%.1f%%", pct))+muted.Render("]"))
	}

	line := prefix + strings.Join(parts, " ")
	if lipgloss.Width(line) > maxW {
		line = ansi.Truncate(line, maxW, ellipsis)
	}
	return line, inputPrompt
}

// ShortWorkdir 将网盘路径压缩为适合状态栏展示的 ~/... 短路径（根目录 '/' 显示为 '~'）。
func ShortWorkdir(workdir string) string {
	return shortHomeWorkdir(workdir, ASCIIOnly())
}

func shortHomeWorkdir(workdir string, ascii bool) string {
	if workdir == "" || workdir == "/" || workdir == "." {
		return "~"
	}
	if !strings.HasPrefix(workdir, "/") {
		workdir = "/" + workdir
	}
	cleaned := path.Clean(workdir)
	if cleaned == "/" || cleaned == "." {
		return "~"
	}
	base := converter.ShortDisplay(path.Base(cleaned), NameShortDisplayNum)
	parent := path.Dir(cleaned)
	if parent == "/" {
		return "~/" + base
	}
	parentBase := converter.ShortDisplay(path.Base(parent), 12)
	if path.Dir(parent) == "/" {
		return "~/" + parentBase + "/" + base
	}
	dots := "…"
	if ascii {
		dots = "..."
	}
	return "~/" + dots + "/" + parentBase + "/" + base
}

func shortBoxedWorkdir(workdir string) string {
	if workdir == "" || workdir == "/" || workdir == "." {
		return "/"
	}
	if !strings.HasPrefix(workdir, "/") {
		workdir = "/" + workdir
	}
	cleaned := path.Clean(workdir)
	if cleaned == "/" || cleaned == "." {
		return "/"
	}
	base := converter.ShortDisplay(path.Base(cleaned), NameShortDisplayNum)
	if path.Dir(cleaned) == "/" {
		return "/" + base
	}
	return ".../" + base
}

func sanitizePromptField(s string) string {
	if s == "" {
		return ""
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	stripped := ansi.Strip(s)
	var sb strings.Builder
	sb.Grow(len(stripped))
	for _, r := range stripped {
		if r == '\t' {
			sb.WriteByte(' ')
			continue
		}
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || isBidiOrFormatNoise(r) {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func isBidiOrFormatNoise(r rune) bool {
	return r == '\u200e' || r == '\u200f' ||
		(r >= '\u202a' && r <= '\u202e') ||
		(r >= '\u2066' && r <= '\u2069') ||
		r == '\ufeff'
}
