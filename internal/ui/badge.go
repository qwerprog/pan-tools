package ui

import (
	"io"
)

// BadgeKind 表示徽章的语义类型。
type BadgeKind int

const (
	BadgeActive BadgeKind = iota
	BadgeOffline
	BadgeInfo
	BadgeWarn
	BadgeError
)

// RenderBadge 渲染统一风格的高对比度状态徽章，例如 [ACTIVE]。
func RenderBadge(w io.Writer, kind BadgeKind, label string, color bool) string {
	r := NewRenderer(w, &color)
	style := r.NewStyle().Bold(true).Padding(0, 1)

	if !color {
		return "[" + label + "]"
	}

	var (
		bg = Palette.Hairline
		fg = Palette.VaultCore
	)
	switch kind {
	case BadgeActive:
		bg = Palette.Success
		fg = Palette.BadgeInk
	case BadgeOffline:
		bg = Palette.Hairline
		fg = Palette.VaultCore
	case BadgeInfo:
		bg = Palette.Primary
		fg = Palette.BadgeInk
	case BadgeWarn:
		bg = Palette.Warn
		fg = Palette.BadgeInk
	case BadgeError:
		bg = Palette.Fail
		fg = Palette.BadgeInk
	}
	return style.Background(bg).Foreground(fg).Render(label)
}
