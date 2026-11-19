package ui

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderQuotaBar 渲染带容量占比着色的现代细线进度条：━━━━━━╸─────  54.2%。
func RenderQuotaBar(w io.Writer, ratio float64, barWidth int, color, ascii bool) string {
	pct := clampRatio(ratio)
	bar := RenderBar(w, pct, barWidth, quotaTone(pct), color, ascii)
	r := NewRenderer(w, &color)
	pctStyle := r.NewStyle().Foreground(quotaTone(pct)).Bold(pct >= 0.75)
	return fmt.Sprintf("%s %s", bar, pctStyle.Render(fmt.Sprintf("%5.1f%%", pct*100)))
}

// RenderBar 渲染 uv / Rich 风格平滑细线进度条主体（不含百分比文本）。
func RenderBar(w io.Writer, ratio float64, barWidth int, fillColor lipgloss.Color, color, ascii bool) string {
	if barWidth < 6 {
		barWidth = 6
	}
	pct := clampRatio(ratio)
	filled := int(math.Round(pct * float64(barWidth)))
	if filled > barWidth {
		filled = barWidth
	}

	r := NewRenderer(w, &color)
	fillStyle := r.NewStyle().Foreground(fillColor)
	trackStyle := r.NewStyle().Foreground(Palette.Hairline)

	if ascii {
		empty := barWidth - filled
		if filled > 0 && empty > 0 {
			return "[" + fillStyle.Render(strings.Repeat("=", filled-1)+">") + trackStyle.Render(strings.Repeat("-", empty)) + "]"
		}
		return "[" + fillStyle.Render(strings.Repeat("=", filled)) + trackStyle.Render(strings.Repeat("-", empty)) + "]"
	}

	empty := barWidth - filled
	if filled == 0 {
		return trackStyle.Render(strings.Repeat("─", barWidth))
	}
	if empty == 0 {
		return fillStyle.Render(strings.Repeat("━", barWidth))
	}
	return fillStyle.Render(strings.Repeat("━", filled-1)+"╸") + trackStyle.Render(strings.Repeat("─", empty))
}

// FormatTransferProgress 生成现代单行上传/下载进度提示字符串（可选携带文件名）。
func FormatTransferProgress(w io.Writer, id, arrow string, ratio float64, done, total, speed, elapsed, left string, color, ascii, tty bool, name ...string) string {
	barWidth := 16
	tone := Palette.Accent
	if arrow == "↓" || arrow == "v" || arrow == "<" {
		tone = Palette.Primary
	}
	bar := RenderBar(w, ratio, barWidth, tone, color, ascii)
	pct := clampRatio(ratio) * 100

	r := NewRenderer(w, &color)
	idStr := r.NewStyle().Foreground(Palette.Muted).Render("[" + id + "]")
	arrowStr := r.NewStyle().Foreground(tone).Bold(true).Render(arrow)
	pctStr := r.NewStyle().Foreground(Palette.VaultCore).Bold(true).Render(fmt.Sprintf("%5.1f%%", pct))
	speedStr := r.NewStyle().Foreground(Palette.Success).Render(speed + "/s")
	sizeStr := r.NewStyle().Foreground(Palette.Subtitle).Render(done + "/" + total)
	meta := r.NewStyle().Foreground(Palette.Muted)

	namePart := ""
	if len(name) > 0 && name[0] != "" {
		shortName := TruncateMiddleASCII(name[0], 22, ascii)
		namePart = r.NewStyle().Foreground(Palette.VaultCore).Render(shortName) + "  "
	}

	sep := "·"
	if ascii {
		sep = "|"
	}

	prefix := "\r"
	if tty {
		prefix = "\r\x1b[2K"
	}

	if left != "" && left != "-" {
		return fmt.Sprintf("%s%s %s %s%s %s  %s %s %s %s",
			prefix,
			idStr, arrowStr, namePart, bar, pctStr,
			sizeStr,
			meta.Render(sep), speedStr,
			meta.Render(fmt.Sprintf("%s ETA %s", sep, left)),
		)
	}
	return fmt.Sprintf("%s%s %s %s%s %s  %s %s %s %s",
		prefix,
		idStr, arrowStr, namePart, bar, pctStr,
		sizeStr,
		meta.Render(sep), speedStr,
		meta.Render(sep+" "+elapsed),
	)
}

func quotaTone(ratio float64) lipgloss.Color {
	switch {
	case ratio >= 0.90:
		return Palette.Fail
	case ratio >= 0.75:
		return Palette.Warn
	default:
		return Palette.Success
	}
}

func clampRatio(ratio float64) float64 {
	if math.IsNaN(ratio) || ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}
