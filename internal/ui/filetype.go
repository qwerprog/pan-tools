package ui

import (
	"io"
	"path"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

// FileCategory 表示文件类型的视觉分类。
type FileCategory int

const (
	FileCategoryDefault FileCategory = iota
	FileCategoryDir
	FileCategoryArchive
	FileCategoryMedia
	FileCategoryCode
	FileCategoryDoc
)

// ClassifyFile 根据目录标志与后缀名返回视觉分类。
func ClassifyFile(name string, isDir bool) FileCategory {
	if isDir {
		return FileCategoryDir
	}
	ext := strings.ToLower(path.Ext(strings.TrimSuffix(name, "/")))
	switch ext {
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".zst", ".iso", ".dmg", ".apk", ".deb", ".rpm":
		return FileCategoryArchive
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v",
		".mp3", ".flac", ".wav", ".aac", ".ogg", ".m4a",
		".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".heic", ".avif":
		return FileCategoryMedia
	case ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".rs", ".c", ".cpp", ".h", ".java", ".kt", ".sh", ".zsh", ".ps1", ".exe", ".bin", ".appimage", ".so", ".dll":
		return FileCategoryCode
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".md", ".txt", ".epub", ".mobi", ".csv", ".json", ".yaml", ".yml", ".toml", ".sql":
		return FileCategoryDoc
	default:
		return FileCategoryDefault
	}
}

// FileCategoryColor 返回对应文件类别的 One Dark 前景色。
func FileCategoryColor(cat FileCategory) lipgloss.Color {
	switch cat {
	case FileCategoryDir:
		return Palette.Primary
	case FileCategoryArchive:
		return Palette.Fail
	case FileCategoryMedia:
		return Palette.Secondary
	case FileCategoryCode:
		return Palette.Success
	case FileCategoryDoc:
		return Palette.Accent
	default:
		return Palette.VaultCore
	}
}

// StyleFileName 按文件类型与终端能力对文件/目录名进行语义着色（不依赖 Nerd Font，零乱码风险）。
func StyleFileName(w io.Writer, display string, isDir bool, color, ascii bool) string {
	if isDir && !strings.HasSuffix(display, "/") {
		display += "/"
	}
	prefix := ""
	if isDir && !ascii {
		prefix = "▸ "
	}
	if !color {
		return prefix + display
	}
	r := NewRenderer(w, &color)
	cat := ClassifyFile(display, isDir)
	st := r.NewStyle().Foreground(FileCategoryColor(cat))
	if isDir {
		st = st.Bold(true)
	}
	return st.Render(prefix + display)
}

// StyleFileSize 按文件体积量级进行层次化着色（目录显示灰阶 '-' 并保留调用方传入的列宽对齐填充，GB 级大文件暖色突出）。
func StyleFileSize(w io.Writer, sizeBytes int64, formatted string, isDir, color bool) string {
	if isDir {
		dirText := formatDirSizeText(formatted)
		if !color {
			return dirText
		}
		return NewRenderer(w, &color).NewStyle().Foreground(Palette.Muted).Render(dirText)
	}
	if !color {
		return formatted
	}
	r := NewRenderer(w, &color)
	switch {
	case sizeBytes >= converter.GB:
		return r.NewStyle().Foreground(Palette.Warn).Bold(true).Render(formatted)
	case sizeBytes >= 100*converter.MB:
		return r.NewStyle().Foreground(Palette.Accent).Render(formatted)
	default:
		return r.NewStyle().Foreground(Palette.Subtitle).Render(formatted)
	}
}

func formatDirSizeText(formatted string) string {
	plain := ansi.Strip(formatted)
	trimmed := strings.TrimSpace(plain)
	if trimmed == "-" {
		return plain
	}
	if w := lipgloss.Width(plain); w > 1 {
		hasLead := strings.HasPrefix(plain, " ")
		hasTrail := strings.HasSuffix(plain, " ")
		switch {
		case trimmed == "":
			return strings.Repeat(" ", w-1) + "-"
		case hasLead && !hasTrail:
			return strings.Repeat(" ", w-1) + "-"
		case hasTrail && !hasLead:
			return "-" + strings.Repeat(" ", w-1)
		case hasLead && hasTrail:
			left := (w - 1) / 2
			return strings.Repeat(" ", left) + "-" + strings.Repeat(" ", w-1-left)
		}
	}
	return "-"
}

// StyleMuted 将次要元信息（如修改时间、序号、连接线）渲染为柔和暗灰。
func StyleMuted(w io.Writer, text string, color bool) string {
	if !color {
		return text
	}
	return NewRenderer(w, &color).NewStyle().Foreground(Palette.Muted).Render(text)
}
