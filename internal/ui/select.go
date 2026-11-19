package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// ErrSelectCanceled 表示用户在交互式选择菜单中主动取消（如 Ctrl+C / Ctrl+D / q）。
var ErrSelectCanceled = errors.New("已取消操作")

// SelectOption 在交互式终端展示支持方向键 (↑/↓ / j/k) 与数字键 (1-9) 的单选菜单，
// 每次仅从 in 读取单字节，绝不预读吞噬后续输入行（保证后续 pcsliner 粘贴凭据不丢字符）。
func SelectOption(in io.Reader, out io.Writer, prompt string, options []string, defaultIdx int) (int, error) {
	if len(options) == 0 {
		return -1, errors.New("没有可选项")
	}
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stderr
	}
	if defaultIdx < 0 || defaultIdx >= len(options) {
		defaultIdx = 0
	}

	colorOn := ColorEnabled(out)
	asciiOn := ASCIIOnly()
	r := NewRenderer(out, &colorOn)

	rawActive := false
	var inFd int
	var oldState *term.State
	if f, ok := in.(*os.File); ok && IsTTY(f) {
		inFd = int(f.Fd())
		if st, err := term.MakeRaw(inFd); err == nil {
			oldState = st
			rawActive = true
		}
	}
	restoreTerm := func() {
		if rawActive && oldState != nil {
			_ = term.Restore(inFd, oldState)
			rawActive = false
		}
	}
	defer restoreTerm()

	outTTY := IsTTY(out) && !strings.EqualFold(os.Getenv("TERM"), "dumb")
	ansiRedraw := rawActive && outTTY
	var outFd int
	if outFile, ok := out.(*os.File); ok && outTTY {
		outFd = int(outFile.Fd())
	}

	if ansiRedraw {
		_, _ = io.WriteString(out, "\x1b[?25l")
		defer func() {
			_, _ = io.WriteString(out, "\x1b[?25h")
		}()
	}

	selected := defaultIdx
	renderedLines := 0

	renderMenu := func() {
		eol := "\n"
		if rawActive {
			eol = "\r\n"
		}
		if ansiRedraw && renderedLines > 0 {
			fmt.Fprintf(out, "\r\x1b[%dA\x1b[J", renderedLines)
		}

		maxLineCols := 0
		if ansiRedraw && outFd > 0 {
			if w, _, szErr := term.GetSize(outFd); szErr == nil && w > 4 {
				maxLineCols = w - 1
			}
		}
		ellipsis := "…"
		if asciiOn {
			ellipsis = "..."
		}
		clampLine := func(s string) string {
			if maxLineCols > 0 && lipgloss.Width(s) > maxLineCols {
				return ansi.Truncate(s, maxLineCols, ellipsis)
			}
			return s
		}

		qIcon := "?"
		cursorGlyph := "❯"
		hintText := "(↑/↓ 或 1-" + fmt.Sprintf("%d", len(options)) + " 选择，Enter 确认，Ctrl+C 取消)"
		compactHint := "(↑/↓/1-" + fmt.Sprintf("%d", len(options)) + ", Enter)"
		if asciiOn {
			cursorGlyph = ">"
			hintText = "(Up/Down or 1-" + fmt.Sprintf("%d", len(options)) + ", Enter, Ctrl+C)"
			compactHint = "(1-" + fmt.Sprintf("%d", len(options)) + ", Enter)"
		}

		qBadge := r.NewStyle().Foreground(Palette.Accent).Bold(true).Render(qIcon)
		titleStyled := r.NewStyle().Bold(true).Render(prompt)
		hintStyled := r.NewStyle().Foreground(Palette.Muted).Render(hintText)
		activeCursorStyle := r.NewStyle().Foreground(Palette.Accent).Bold(true)
		activeItemStyle := r.NewStyle().Foreground(Palette.Primary).Bold(true)
		inactiveNumStyle := r.NewStyle().Foreground(Palette.Muted)

		headerLine := fmt.Sprintf("%s %s %s", qBadge, titleStyled, hintStyled)
		if maxLineCols > 0 && lipgloss.Width(headerLine) > maxLineCols {
			headerLine = fmt.Sprintf("%s %s %s", qBadge, titleStyled, r.NewStyle().Foreground(Palette.Muted).Render(compactHint))
		}
		headerLine = clampLine(headerLine)

		var sb strings.Builder
		sb.WriteString(headerLine)
		sb.WriteString(eol)
		lines := 1
		for i, opt := range options {
			numLabel := fmt.Sprintf("%d.", i+1)
			var itemLine string
			if i == selected {
				itemLine = fmt.Sprintf("  %s %s",
					activeCursorStyle.Render(cursorGlyph),
					activeItemStyle.Render(numLabel+" "+opt),
				)
			} else {
				itemLine = fmt.Sprintf("    %s %s",
					inactiveNumStyle.Render(numLabel),
					opt,
				)
			}
			sb.WriteString(clampLine(itemLine))
			sb.WriteString(eol)
			lines++
		}
		_, _ = io.WriteString(out, sb.String())
		renderedLines = lines
	}

	finishConfirm := func() (int, error) {
		if ansiRedraw && renderedLines > 0 {
			fmt.Fprintf(out, "\r\x1b[%dA\x1b[J", renderedLines)
			renderedLines = 0
		}
		restoreTerm()

		okIcon := "✓"
		if asciiOn {
			okIcon = "[OK]"
		}
		okBadge := r.NewStyle().Foreground(Palette.Success).Bold(true).Render(okIcon)
		chosen := r.NewStyle().Foreground(Palette.Primary).Bold(true).Render(options[selected])
		fmt.Fprintf(out, "%s %s: %s\n", okBadge, prompt, chosen)
		return selected, nil
	}

	finishCancel := func() (int, error) {
		if ansiRedraw && renderedLines > 0 {
			fmt.Fprintf(out, "\r\x1b[%dA\x1b[J", renderedLines)
			renderedLines = 0
		}
		restoreTerm()
		return -1, ErrSelectCanceled
	}

	renderMenu()

	var buf [1]byte
	sawCR := false
	for {
		n, err := in.Read(buf[:])
		if n == 0 {
			if errors.Is(err, io.EOF) && sawCR {
				return finishConfirm()
			}
			if err != nil {
				return finishCancel()
			}
			continue
		}

		b := buf[0]
		if b != '\n' {
			sawCR = false
		}

		switch {
		case b == 0x03 || b == 0x04 || b == 'q' || b == 'Q':
			return finishCancel()
		case b == '\r':
			if rawActive || errors.Is(err, io.EOF) {
				return finishConfirm()
			}
			sawCR = true
		case b == '\n':
			return finishConfirm()
		case b == 'k' || b == 'K':
			selected = (selected - 1 + len(options)) % len(options)
			if ansiRedraw {
				renderMenu()
			}
		case b == 'j' || b == 'J':
			selected = (selected + 1) % len(options)
			if ansiRedraw {
				renderMenu()
			}
		case b >= '1' && b <= '9':
			idx := int(b - '1')
			if idx < len(options) {
				selected = idx
				if ansiRedraw {
					renderMenu()
				}
			}
		case b == 0x1b:
			// 解析 ANSI 方向键序列 (\x1b[A / \x1b[B / \x1bOA / \x1bOB / \x1b[H / \x1b[F)
			var seq [1]byte
			if n1, err1 := in.Read(seq[:]); n1 == 0 {
				_ = err1
				return finishCancel()
			}
			switch seq[0] {
			case 0x1b, 0x03, 0x04, 'q', 'Q':
				return finishCancel()
			case '[', 'O':
				var code [1]byte
				if n2, err2 := in.Read(code[:]); n2 == 0 {
					_ = err2
					return finishCancel()
				}
				c := code[0]
				switch c {
				case 'A':
					selected = (selected - 1 + len(options)) % len(options)
					if ansiRedraw {
						renderMenu()
					}
				case 'B':
					selected = (selected + 1) % len(options)
					if ansiRedraw {
						renderMenu()
					}
				case 'H':
					selected = 0
					if ansiRedraw {
						renderMenu()
					}
				case 'F':
					selected = len(options) - 1
					if ansiRedraw {
						renderMenu()
					}
				default:
					// 消费完整的多字节 CSI 参数序列（如 \x1b[1~ / \x1b[5~），防止尾字符污染输入
					if seq[0] == '[' && (c < 0x40 || c > 0x7e) {
						for {
							var tail [1]byte
							if nt, errT := in.Read(tail[:]); nt == 0 || errT != nil || (tail[0] >= 0x40 && tail[0] <= 0x7e) {
								break
							}
						}
					}
				}
			}
		}
	}
}
