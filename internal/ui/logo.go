package ui

import (
	"io"
	"strings"
)

// RenderLogoLines 返回无缝箱线几何徽标（任意终端行高均不断裂，最大列宽 <= 50 列）。
func RenderLogoLines(w io.Writer, color, ascii bool) []string {
	if ascii {
		return renderASCIILogoLines(w, color)
	}
	return renderUnicodeLogoLines(w, color)
}

// RenderLogo 返回多行拼接后的 pan-tools 徽标字符串。
func RenderLogo(w io.Writer, color, ascii bool) string {
	return strings.Join(RenderLogoLines(w, color, ascii), "\n")
}

func renderUnicodeLogoLines(w io.Writer, color bool) []string {
	r := NewRenderer(w, &color)
	frame := r.NewStyle().Foreground(Palette.VaultFrame)
	core := r.NewStyle().Foreground(Palette.Accent).Bold(true)
	word := r.NewStyle().Foreground(Palette.Primary).Bold(true)
	tag := r.NewStyle().Foreground(Palette.Secondary)
	rule := r.NewStyle().Foreground(Palette.Hairline)
	sub := r.NewStyle().Foreground(Palette.Subtitle)

	return []string{
		frame.Render("╭──") + core.Render("◈") + frame.Render("──╮") + "  " + word.Render(BrandName) + " " + tag.Render("· "+BrandTag),
		frame.Render("│ ╭─╮ │") + "  " + rule.Render("────────────────────────────────────"),
		frame.Render("╰─┴─┴─╯") + "  " + sub.Render(Tagline),
	}
}

func renderASCIILogoLines(w io.Writer, color bool) []string {
	r := NewRenderer(w, &color)
	frame := r.NewStyle().Foreground(Palette.VaultFrame)
	core := r.NewStyle().Foreground(Palette.Accent).Bold(true)
	word := r.NewStyle().Foreground(Palette.Primary).Bold(true)
	tag := r.NewStyle().Foreground(Palette.Secondary)
	rule := r.NewStyle().Foreground(Palette.Hairline)
	sub := r.NewStyle().Foreground(Palette.Subtitle)

	return []string{
		frame.Render("+--") + core.Render("*") + frame.Render("--+") + "  " + word.Render(BrandName) + " " + tag.Render("- PCS CLI"),
		frame.Render("| +-+ |") + "  " + rule.Render("------------------------------------"),
		frame.Render("+-^-^-+") + "  " + sub.Render("Fast Multi-Thread Cloud Storage CLI"),
	}
}
