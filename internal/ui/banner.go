package ui

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
	"github.com/rivo/uniseg"
)

// MaskUID 对百度 UID 中间数字进行部分脱敏（例如 17596386318481 -> 175****481）。
func MaskUID(uid uint64) string {
	if uid == 0 {
		return "0"
	}
	s := strconv.FormatUint(uid, 10)
	n := len(s)
	switch {
	case n >= 8:
		return s[:3] + "****" + s[n-3:]
	case n >= 5:
		return s[:2] + "***" + s[n-2:]
	case n >= 2:
		return s[:1] + "**"
	default:
		return "***"
	}
}

// BannerInfo 包含启动欢迎横幅渲染所需的会话状态与终端环境参数。
type BannerInfo struct {
	AppName    string
	BrandName  string
	Version    string
	Platform   string
	LoggedIn   bool
	UserName   string
	UID        uint64
	VIPLabel   string
	IsVIP      bool
	IsSVIP     bool
	Workdir    string
	QuotaTotal int64
	QuotaUsed  int64
	HasQuota   bool
	TermWidth  int
	Color      *bool
	ASCII      *bool
}

// RenderBanner 根据终端宽度自适应选择宽屏双列（Wide）、标准 80 列堆叠（Standard）
// 或窄屏无边框模式（Compact）渲染启动欢迎横幅。
func RenderBanner(w io.Writer, info BannerInfo) string {
	colorOn := ColorEnabled(w)
	if info.Color != nil {
		colorOn = *info.Color
	}
	asciiOn := ASCIIOnly()
	if info.ASCII != nil {
		asciiOn = *info.ASCII
	}
	width := info.TermWidth
	if width <= 0 {
		width = TerminalWidth()
	}

	normalizeBannerInfo(&info)

	switch {
	case width >= 112:
		return renderWideBanner(w, info, width, colorOn, asciiOn)
	case width >= 74:
		return renderStandardBanner(w, info, width, colorOn, asciiOn)
	default:
		return renderCompactBanner(w, info, width, colorOn, asciiOn)
	}
}

func normalizeBannerInfo(info *BannerInfo) {
	if info.AppName == "" {
		info.AppName = AppName
	}
	if info.BrandName == "" {
		info.BrandName = BrandName
	}
	if info.Workdir == "" {
		info.Workdir = "/"
	}
}

func renderWideBanner(w io.Writer, info BannerInfo, termWidth int, color, ascii bool) string {
	r := NewRenderer(w, &color)
	innerLeftWidth := 48
	innerRightWidth := termWidth - 58
	if innerRightWidth < 52 {
		innerRightWidth = 52
	}
	if innerRightWidth > 64 {
		innerRightWidth = 64
	}

	logoLines := RenderLogoLines(w, color, ascii)
	muted := r.NewStyle().Foreground(Palette.Muted)
	accent := r.NewStyle().Foreground(Palette.Accent)
	verStyle := r.NewStyle().Foreground(Palette.VaultCore).Bold(true)

	var leftLines []string
	leftLines = append(leftLines, logoLines...)
	hintSep := " · "
	if ascii {
		hintSep = " | "
	}
	leftLines = append(leftLines,
		"",
		fmt.Sprintf("%s %s  %s %s",
			muted.Render("版本"), verStyle.Render(info.Version),
			muted.Render("平台"), muted.Render(info.Platform),
		),
		fmt.Sprintf("%s %s",
			muted.Render("提示"),
			accent.Render("输入 'ui' 打开全屏浏览器"+hintSep+"'help' 查看命令"),
		),
	)
	leftBlock := padBlockLines(leftLines, innerLeftWidth, ascii)

	rightLines := buildSessionLines(w, info, innerRightWidth, color, ascii, true)
	rightBlock := padBlockLines(rightLines, innerRightWidth, ascii)

	border := lipgloss.RoundedBorder()
	if ascii {
		border = lipgloss.ASCIIBorder()
	}

	leftH := lipgloss.Height(leftBlock)
	rightH := lipgloss.Height(rightBlock)
	maxH := leftH
	if rightH > maxH {
		maxH = rightH
	}

	sepChar := "│"
	if ascii {
		sepChar = "|"
	}
	sepColLines := make([]string, maxH)
	sepStyled := r.NewStyle().Foreground(Palette.Hairline).Render(" " + sepChar + " ")
	for i := range sepColLines {
		sepColLines[i] = sepStyled
	}
	sepCol := strings.Join(sepColLines, "\n")

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		r.NewStyle().Width(innerLeftWidth).Height(maxH).Render(leftBlock),
		sepCol,
		r.NewStyle().Width(innerRightWidth).Height(maxH).Render(rightBlock),
	)

	box := r.NewStyle().
		Border(border).
		BorderForeground(Palette.Hairline).
		Padding(0, 1).
		Render(content)

	return indentBlock(box, "  ")
}

func renderStandardBanner(w io.Writer, info BannerInfo, termWidth int, color, ascii bool) string {
	r := NewRenderer(w, &color)
	outerWidth := 76
	if termWidth-2 < outerWidth {
		outerWidth = termWidth - 2
	}
	innerWidth := outerWidth - 4

	border := lipgloss.RoundedBorder()
	if ascii {
		border = lipgloss.ASCIIBorder()
	}

	ruleChar := "─"
	if ascii {
		ruleChar = "-"
	}
	divider := r.NewStyle().Foreground(Palette.Hairline).Render(strings.Repeat(ruleChar, innerWidth))

	var lines []string
	for _, l := range RenderLogoLines(w, color, ascii) {
		lines = append(lines, ansi.Truncate(l, innerWidth, ""))
	}
	lines = append(lines, divider)
	lines = append(lines, buildSessionLines(w, info, innerWidth, color, ascii, false)...)

	body := strings.Join(lines, "\n")
	box := r.NewStyle().
		Border(border).
		BorderForeground(Palette.Hairline).
		Padding(0, 1).
		Width(innerWidth + 2).
		Render(body)

	return indentBlock(box, "  ")
}

func renderCompactBanner(w io.Writer, info BannerInfo, termWidth int, color, ascii bool) string {
	r := NewRenderer(w, &color)
	title := r.NewStyle().Foreground(Palette.Primary).Bold(true).Render(info.BrandName)
	muted := r.NewStyle().Foreground(Palette.Muted)
	ellipsis := "…"
	sep := " · "
	if ascii {
		ellipsis = "..."
		sep = " | "
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("  %s %s %s", title, info.Version, muted.Render("("+info.Platform+")")))

	if info.LoggedIn && info.UserName != "" {
		accLine := fmt.Sprintf("  账号 %s (%s)", info.UserName, MaskUID(info.UID))
		if info.HasQuota && info.QuotaTotal > 0 {
			pct := 100 * float64(info.QuotaUsed) / float64(info.QuotaTotal)
			accLine += fmt.Sprintf("%s%.1f%%", sep, pct)
		}
		lines = append(lines, ansi.Truncate(accLine, termWidth-1, ellipsis))
		lines = append(lines, ansi.Truncate("  目录 "+info.Workdir, termWidth-1, ellipsis))
	} else {
		warn := r.NewStyle().Foreground(Palette.Warn).Render("未登录")
		lines = append(lines, "  账号 "+warn+" (输入 'login' 登录)")
	}
	lines = append(lines, ansi.Truncate(muted.Render("  ui 交互浏览"+sep+"ls 列表"+sep+"help 帮助"+sep+"exit 退出"), termWidth-1, ellipsis))
	return strings.Join(lines, "\n")
}

func buildSessionLines(w io.Writer, info BannerInfo, maxWidth int, color, ascii, wideRight bool) []string {
	r := NewRenderer(w, &color)
	muted := r.NewStyle().Foreground(Palette.Muted)
	ok := r.NewStyle().Foreground(Palette.Success).Bold(true)
	warn := r.NewStyle().Foreground(Palette.Warn).Bold(true)
	accent := r.NewStyle().Foreground(Palette.Accent)
	cmdKey := r.NewStyle().Foreground(Palette.Primary)

	onlineIcon, offlineIcon := "●", "○"
	ellipsis := "…"
	sep := " · "
	if ascii {
		onlineIcon, offlineIcon = "*", "o"
		ellipsis = "..."
		sep = " | "
	}

	firstLine := fmt.Sprintf("%s  %-16s  %s  %s",
		muted.Render("版本"),
		r.NewStyle().Foreground(Palette.VaultCore).Render(info.Version),
		muted.Render("平台"),
		muted.Render(info.Platform),
	)
	if wideRight {
		firstLine = r.NewStyle().Foreground(Palette.Subtitle).Bold(true).Render("当前会话状态")
	}

	var accLine, workdirLine, quotaLine string
	if info.LoggedIn && info.UserName != "" {
		userBadge := ok.Render(onlineIcon + " " + info.UserName)
		if info.VIPLabel != "" {
			kind := BadgeOffline
			if info.IsSVIP {
				kind = BadgeWarn
			} else if info.IsVIP {
				kind = BadgeInfo
			}
			userBadge += " " + RenderBadge(w, kind, info.VIPLabel, color)
		}
		accLine = fmt.Sprintf("%s  %s %s",
			muted.Render("账号"),
			userBadge,
			muted.Render(fmt.Sprintf("UID %s", MaskUID(info.UID))),
		)
		workdirLine = fmt.Sprintf("%s  %s",
			muted.Render("目录"),
			accent.Render(TruncateMiddleASCII(info.Workdir, maxWidth-8, ascii)),
		)
		if info.HasQuota && info.QuotaTotal > 0 {
			ratio := float64(info.QuotaUsed) / float64(info.QuotaTotal)
			bar := RenderQuotaBar(w, ratio, 14, color, ascii)
			usedStr := converter.ConvertFileSize(info.QuotaUsed, 2)
			totalStr := converter.ConvertFileSize(info.QuotaTotal, 2)
			quotaLine = fmt.Sprintf("%s  %s %s",
				muted.Render("空间"),
				bar,
				muted.Render(fmt.Sprintf("%s / %s", usedStr, totalStr)),
			)
		} else {
			bar := RenderBar(w, 0, 14, Palette.Muted, color, ascii)
			quotaLine = fmt.Sprintf("%s  %s %s",
				muted.Render("空间"),
				bar,
				muted.Render("输入 'quota' 刷新"),
			)
		}
	} else {
		accLine = fmt.Sprintf("%s  %s",
			muted.Render("账号"),
			warn.Render(offlineIcon+" 未登录"+sep+"输入 'login' 登录百度网盘"),
		)
		workdirLine = fmt.Sprintf("%s  %s", muted.Render("目录"), muted.Render("-"))
		bar := RenderBar(w, 0, 14, Palette.Muted, color, ascii)
		quotaLine = fmt.Sprintf("%s  %s %s", muted.Render("空间"), bar, muted.Render("登录后自动同步"))
	}

	shortcutLine := fmt.Sprintf("%s  %s 交互界面%s%s 列表%s%s 下载%s%s 上传%s%s 补全",
		muted.Render("快捷"),
		cmdKey.Render("ui"), sep,
		cmdKey.Render("ls"), sep,
		cmdKey.Render("d"), sep,
		cmdKey.Render("u"), sep,
		cmdKey.Render("Tab"),
	)

	out := []string{firstLine, accLine, workdirLine, quotaLine, shortcutLine}
	for i := range out {
		out[i] = ansi.Truncate(out[i], maxWidth, ellipsis)
	}
	return out
}

// TruncateMiddle 从中间省略过长路径或字符串，保留前缀和末尾文件名。
func TruncateMiddle(s string, maxWidth int) string {
	return TruncateMiddleASCII(s, maxWidth, ASCIIOnly())
}

// TruncateMiddleASCII 按显式 ascii 标志从中间省略过长路径或字符串（严格按 UAX #29 字素簇切分与计宽，避免拆裂组合音标/VS16/ZWJ Emoji 导致列宽越界）。
func TruncateMiddleASCII(s string, maxWidth int, ascii bool) string {
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	ellipsis := "…"
	if ascii {
		ellipsis = "..."
	}
	ew := lipgloss.Width(ellipsis)
	if maxWidth <= ew+2 {
		return ansi.Truncate(s, maxWidth, ellipsis)
	}

	plain := ansi.Strip(s)
	type grapheme struct {
		text  string
		width int
	}
	var clusters []grapheme
	state := -1
	rest := plain
	for len(rest) > 0 {
		var g string
		var w int
		g, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
		clusters = append(clusters, grapheme{text: g, width: w})
	}

	budget := maxWidth - ew
	headBudget := budget / 2
	tailBudget := budget - headBudget

	var head strings.Builder
	headW := 0
	headEnd := 0
	for i, c := range clusters {
		if headW+c.width > headBudget {
			break
		}
		head.WriteString(c.text)
		headW += c.width
		headEnd = i + 1
	}

	tailStart := len(clusters)
	tailW := 0
	for i := len(clusters) - 1; i >= headEnd; i-- {
		if tailW+clusters[i].width > tailBudget {
			break
		}
		tailW += clusters[i].width
		tailStart = i
	}
	for tailStart < len(clusters) && clusters[tailStart].width == 0 {
		tailStart++
	}

	var out strings.Builder
	out.WriteString(head.String())
	out.WriteString(ellipsis)
	for i := tailStart; i < len(clusters); i++ {
		out.WriteString(clusters[i].text)
	}
	res := out.String()
	if lipgloss.Width(res) > maxWidth {
		return ansi.Truncate(res, maxWidth, ellipsis)
	}
	return res
}

func padBlockLines(lines []string, targetWidth int, ascii bool) string {
	ellipsis := "…"
	if ascii {
		ellipsis = "..."
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		truncated := ansi.Truncate(line, targetWidth, ellipsis)
		w := lipgloss.Width(truncated)
		if w < targetWidth {
			out[i] = truncated + strings.Repeat(" ", targetWidth-w)
		} else {
			out[i] = truncated
		}
	}
	return strings.Join(out, "\n")
}

func indentBlock(block, indent string) string {
	if block == "" {
		return ""
	}
	lines := strings.Split(block, "\n")
	for i := range lines {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}
