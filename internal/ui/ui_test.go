package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

func boolPtr(v bool) *bool { return &v }

func TestRenderLogoWidthAndFallback(t *testing.T) {
	var buf bytes.Buffer

	for _, tc := range []struct {
		name  string
		color bool
		ascii bool
	}{
		{"unicode_mono", false, false},
		{"unicode_color", true, false},
		{"ascii_mono", false, true},
		{"ascii_color", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := RenderLogoLines(&buf, tc.color, tc.ascii)
			if len(lines) == 0 {
				t.Fatal("expected non-empty logo lines")
			}
			for i, line := range lines {
				w := lipgloss.Width(line)
				if w > 80 {
					t.Fatalf("line %d exceeds 80 cols: got %d (%q)", i, w, line)
				}
				if !tc.color && strings.Contains(line, "\x1b") {
					t.Fatalf("line %d contains ANSI escape when color=false: %q", i, line)
				}
				if tc.ascii && !tc.color {
					for _, r := range line {
						if r > unicode.MaxASCII {
							t.Fatalf("ascii fallback line %d contains non-ASCII rune %U: %q", i, r, line)
						}
					}
				}
			}
		})
	}
}

func TestRenderBannerResponsiveWidths(t *testing.T) {
	var buf bytes.Buffer
	cases := []struct {
		name      string
		termWidth int
		loggedIn  bool
		ascii     bool
		color     bool
	}{
		{"wide_logged_in", 120, true, false, true},
		{"standard_80_logged_in", 80, true, false, false},
		{"standard_80_logged_out", 80, false, false, false},
		{"standard_80_ascii", 80, true, true, false},
		{"compact_60_logged_in", 60, true, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := RenderBanner(&buf, BannerInfo{
				AppName:    "pant",
				BrandName:  "pan-tools",
				Version:    "v4.0.2-dev",
				Platform:   "linux/amd64",
				LoggedIn:   tc.loggedIn,
				UserName:   "test_user",
				UID:        123456,
				VIPLabel:   "SVIP 10",
				IsVIP:      true,
				IsSVIP:     true,
				Workdir:    "/我的资源/项目资料",
				QuotaTotal: 2 << 40,
				QuotaUsed:  1 << 40,
				HasQuota:   true,
				TermWidth:  tc.termWidth,
				Color:      boolPtr(tc.color),
				ASCII:      boolPtr(tc.ascii),
			})
			if out == "" {
				t.Fatal("expected non-empty banner output")
			}
			for i, line := range strings.Split(out, "\n") {
				if w := lipgloss.Width(line); w > tc.termWidth {
					t.Fatalf("banner line %d width %d exceeds termWidth %d: %q", i, w, tc.termWidth, line)
				}
				if !tc.color && strings.Contains(line, "\x1b") {
					t.Fatalf("unexpected ANSI escape on uncolored banner line %d", i)
				}
			}
		})
	}
}

func TestFormatPromptLinerSafety(t *testing.T) {
	defer RestoreOverrides(UIOverrides{})
	RestoreOverrides(UIOverrides{})
	t.Setenv(EnvPromptStyle, "")

	var buf bytes.Buffer
	for _, ascii := range []bool{false, true} {
		for _, color := range []bool{false, true} {
			header, inputPrompt := FormatPrompt(&buf, PromptContext{
				Brand:      "pan-tools",
				Style:      PromptStyleStarship,
				UserName:   "chapman",
				VIPLabel:   "SVIP 10",
				IsVIP:      true,
				IsSVIP:     true,
				Workdir:    "/文档/工作/架构设计",
				LoggedIn:   true,
				QuotaRatio: 0.542,
				QuotaUsed:  1100 * (1 << 30),
				QuotaTotal: 2048 * (1 << 30),
				HasQuota:   true,
				TermWidth:  100,
				Color:      boolPtr(color),
				ASCII:      boolPtr(ascii),
			})
			if !strings.Contains(header, "chapman") || !strings.Contains(header, "SVIP 10") || !strings.Contains(header, "~") || !strings.Contains(header, "架构设计") || !strings.Contains(header, "54.2%") || !strings.Contains(header, "/2.0TB") {
				t.Fatalf("header missing expected Starship right-aligned context: %q", header)
			}
			if w := lipgloss.Width(header); w != 99 {
				t.Fatalf("expected right-aligned header width 99 on 100-col terminal, got %d: %q", w, header)
			}
			if !color && strings.Contains(header, "\x1b") {
				t.Fatalf("header contains ANSI escape when color=false: %q", header)
			}
			if inputPrompt == "" {
				t.Fatal("inputPrompt must not be empty")
			}
			// 复现 peterh/liner@v1.2.2/line.go:605 严格校验，确保绝不触发 ErrInvalidPrompt。
			for _, r := range inputPrompt {
				if unicode.Is(unicode.C, r) {
					t.Fatalf("inputPrompt contains control rune %U (breaks liner)", r)
				}
			}
		}
	}
}

func TestFormatPromptBoxedStylePrecedenceAndEdgeCases(t *testing.T) {
	defer RestoreOverrides(UIOverrides{})
	t.Setenv(EnvPromptStyle, "")

	var buf bytes.Buffer

	// 1. Exact boxed output (Before style) vs starship output (After style).
	boxedHeader, boxedInput := FormatPrompt(&buf, PromptContext{
		Brand:      "pan-tools",
		Style:      PromptStyleBoxed,
		UserName:   "alice",
		VIPLabel:   "SVIP",
		IsVIP:      true,
		IsSVIP:     true,
		Workdir:    "/工作/架构设计",
		LoggedIn:   true,
		QuotaRatio: 0.542,
		QuotaUsed:  1126 * (1 << 30),
		QuotaTotal: 2048 * (1 << 30),
		HasQuota:   true,
		TermWidth:  100,
		Color:      boolPtr(false),
		ASCII:      boolPtr(false),
	})
	wantBoxed := "╭─ pan-tools ▏ ● alice [VIP: SVIP] [dir: .../架构设计] [quota: 54.2%]"
	if boxedHeader != wantBoxed {
		t.Fatalf("boxed header = %q, want %q", boxedHeader, wantBoxed)
	}
	if boxedInput != "╰─❯ " {
		t.Fatalf("boxed inputPrompt = %q, want %q", boxedInput, "╰─❯ ")
	}

	// 2. Strict 7-bit ASCII compliance & liner safety across all style/color/ascii/login combinations.
	for _, style := range []PromptStyle{PromptStyleStarship, PromptStyleBoxed} {
		for _, color := range []bool{false, true} {
			for _, ascii := range []bool{false, true} {
				for _, loggedIn := range []bool{false, true} {
					hdr, inp := FormatPrompt(&buf, PromptContext{
						Brand:      "pan-tools",
						Style:      style,
						UserName:   "alice",
						VIPLabel:   "SVIP",
						IsVIP:      true,
						IsSVIP:     true,
						Workdir:    "/work/arch",
						LoggedIn:   loggedIn,
						QuotaRatio: 0.91,
						QuotaUsed:  910 * (1 << 30),
						QuotaTotal: 1000 * (1 << 30),
						HasQuota:   true,
						TermWidth:  80,
						Color:      boolPtr(color),
						ASCII:      boolPtr(ascii),
					})
					if !color && strings.Contains(hdr, "\x1b") {
						t.Fatalf("style=%s ascii=%v loggedIn=%v leaked ANSI when color=false: %q", style, ascii, loggedIn, hdr)
					}
					if strings.Contains(inp, "\x1b") || inp == "" {
						t.Fatalf("style=%s ascii=%v invalid inputPrompt: %q", style, ascii, inp)
					}
					for _, r := range inp {
						if unicode.Is(unicode.C, r) {
							t.Fatalf("style=%s ascii=%v inputPrompt contains control rune %U", style, ascii, r)
						}
						if ascii && r > unicode.MaxASCII {
							t.Fatalf("style=%s ascii=true inputPrompt contains non-ASCII rune %U: %q", style, r, inp)
						}
					}
					if ascii && loggedIn {
						for _, r := range hdr {
							if r > unicode.MaxASCII {
								t.Fatalf("style=%s color=%v ascii=true header contains non-ASCII rune %U: %q", style, color, r, hdr)
							}
						}
					}
				}
			}
		}
	}

	// 3. Boxed workdir edge cases, missing VIP/quota, and narrow-terminal clamping.
	rootHdr, _ := FormatPrompt(&buf, PromptContext{
		Brand:     "pan-tools",
		Style:     PromptStyleBoxed,
		UserName:  "alice",
		Workdir:   "/",
		LoggedIn:  true,
		TermWidth: 80,
		Color:     boolPtr(false),
		ASCII:     boolPtr(false),
	})
	if rootHdr != "╭─ pan-tools ▏ ● alice [dir: /]" {
		t.Fatalf("unexpected root boxed header without VIP/quota: %q", rootHdr)
	}

	topDirHdr, _ := FormatPrompt(&buf, PromptContext{
		Brand:     "pan-tools",
		Style:     PromptStyleBoxed,
		UserName:  "alice",
		Workdir:   "/架构设计",
		LoggedIn:  true,
		TermWidth: 80,
		Color:     boolPtr(false),
		ASCII:     boolPtr(false),
	})
	if topDirHdr != "╭─ pan-tools ▏ ● alice [dir: /架构设计]" {
		t.Fatalf("unexpected depth-1 boxed header: %q", topDirHdr)
	}

	narrowHdr, narrowInp := FormatPrompt(&buf, PromptContext{
		Brand:      "pan-tools",
		Style:      PromptStyleBoxed,
		UserName:   "alice",
		VIPLabel:   "SVIP 10",
		Workdir:    "/work/very_long_directory_name_2026",
		LoggedIn:   true,
		QuotaRatio: 0.542,
		HasQuota:   true,
		TermWidth:  36,
		Color:      boolPtr(true),
		ASCII:      boolPtr(true),
	})
	if w := lipgloss.Width(narrowHdr); w > 35 {
		t.Fatalf("narrow boxed header width %d exceeds 35: %q", w, narrowHdr)
	}
	if narrowInp != "+-> " {
		t.Fatalf("expected '+-> ' for ASCII boxed inputPrompt, got %q", narrowInp)
	}
	for _, r := range narrowHdr {
		if r > unicode.MaxASCII {
			t.Fatalf("narrow colored ASCII boxed header leaked non-ASCII rune %U: %q", r, narrowHdr)
		}
	}

	// Relative '..' segments, ultra-narrow TermWidth (<20), and control runes in fields.
	if dotStarHdr, _ := FormatPrompt(&buf, PromptContext{
		Brand:     "pan-tools",
		Style:     PromptStyleStarship,
		UserName:  "alice\n\x1b[2J",
		VIPLabel:  "SVIP\r\n",
		Workdir:   "..",
		LoggedIn:  true,
		TermWidth: 80,
		Color:     boolPtr(false),
		ASCII:     boolPtr(false),
	}); dotStarHdr != "alice  [SVIP]  ~" {
		t.Fatalf("unexpected starship header for '..' workdir and control runes: %q", dotStarHdr)
	}
	if dotBoxHdr, _ := FormatPrompt(&buf, PromptContext{
		Brand:     "pan-tools\n",
		Style:     PromptStyleBoxed,
		UserName:  "alice\r\n",
		Workdir:   "../架构设计",
		LoggedIn:  true,
		TermWidth: 80,
		Color:     boolPtr(false),
		ASCII:     boolPtr(false),
	}); dotBoxHdr != "╭─ pan-tools ▏ ● alice [dir: /架构设计]" {
		t.Fatalf("unexpected boxed header for '../架构设计' workdir and control runes: %q", dotBoxHdr)
	}
	tinyHdr, _ := FormatPrompt(&buf, PromptContext{
		Brand:     "pan-tools",
		Style:     PromptStyleBoxed,
		UserName:  "alice",
		Workdir:   "/work/arch",
		LoggedIn:  true,
		TermWidth: 16,
		Color:     boolPtr(false),
		ASCII:     boolPtr(true),
	})
	if w := lipgloss.Width(tinyHdr); w > 15 {
		t.Fatalf("expected TermWidth=16 header clamped to <=15 cols, got %d: %q", w, tinyHdr)
	}

	// 4. ParsePromptStyle & EffectivePromptStyle precedence (CLI > Env > Config > Default).
	if got, err := ParsePromptStyle("  BOXED "); err != nil || got != PromptStyleBoxed {
		t.Fatalf("ParsePromptStyle('  BOXED ') = (%q, %v), want ('boxed', nil)", got, err)
	}
	if got, err := ParsePromptStyle("minimal"); err != nil || got != PromptStyleStarship {
		t.Fatalf("ParsePromptStyle('minimal') = (%q, %v), want ('starship', nil)", got, err)
	}
	if _, err := ParsePromptStyle("bogus_style"); err == nil {
		t.Fatal("expected error from ParsePromptStyle('bogus_style')")
	}
	if got := NormalizePromptStyle("bogus_style"); got != PromptStyleStarship {
		t.Fatalf("NormalizePromptStyle('bogus_style') = %q, want 'starship'", got)
	}

	if got := EffectivePromptStyle("boxed"); got != PromptStyleBoxed {
		t.Fatalf("EffectivePromptStyle('boxed') = %q, want 'boxed'", got)
	}
	t.Setenv(EnvPromptStyle, "invalid_env_val")
	if got := EffectivePromptStyle("boxed"); got != PromptStyleBoxed {
		t.Fatalf("invalid PANT_PROMPT_STYLE should fall back to configured 'boxed', got %q", got)
	}
	t.Setenv(EnvPromptStyle, "starship")
	if got := EffectivePromptStyle("boxed"); got != PromptStyleStarship {
		t.Fatalf("PANT_PROMPT_STYLE='starship' should override configured 'boxed', got %q", got)
	}
	SetPromptStyleOverride("boxed")
	if got := EffectivePromptStyle("starship"); got != PromptStyleBoxed {
		t.Fatalf("CLI override 'boxed' should override PANT_PROMPT_STYLE='starship', got %q", got)
	}
	snap := SnapshotOverrides()
	SetPromptStyleOverride("")
	RestoreOverrides(snap)
	if GetPromptStyleOverride() != "boxed" {
		t.Fatalf("RestoreOverrides failed to restore PromptStyle override, got %q", GetPromptStyleOverride())
	}
}

func TestRenderTableWidthClampingAndASCII(t *testing.T) {
	var buf bytes.Buffer
	rows := [][]string{
		{"0", "9182301988123", "266719", "1.42 GB", "2026-11-14 18:20:00", "2026-11-15 11:45:00", "9e107d9d372bb6826bd81d3542a419d6", "这是一个非常非常长且包含中文字符的测试文件名_2026_final_release.tar.gz"},
		{"1", "9182302400001", "266719", "-", "2026-11-15 09:12:00", "2026-11-15 09:12:00", "", "Documents/"},
	}
	headers := []string{"#", "fs_id", "app_id", "文件大小", "创建日期", "修改日期", "md5(截图请打码)", "文件(目录)"}

	unicodeOut := RenderTable(&buf, TableConfig{
		Headers:  headers,
		Rows:     rows,
		MaxWidth: 80,
		Color:    boolPtr(false),
		ASCII:    boolPtr(false),
	})
	if !strings.Contains(unicodeOut, "╭") || !strings.Contains(unicodeOut, "╯") {
		t.Fatalf("expected rounded unicode borders in output:\n%s", unicodeOut)
	}
	for i, line := range strings.Split(strings.TrimSuffix(unicodeOut, "\n"), "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Fatalf("table line %d width %d exceeds maxWidth 80: %q", i, w, line)
		}
	}

	asciiHeaders := []string{"#", "fs_id", "app_id", "size", "created_at", "modified_at", "md5", "path"}
	asciiRows := [][]string{
		{"0", "9182301988123", "266719", "1.42 GB", "2026-11-14 18:20:00", "2026-11-15 11:45:00", "9e107d9d372bb6826bd81d3542a419d6", "very_long_ascii_archive_filename_2026_final_release.tar.gz"},
		{"1", "9182302400001", "266719", "-", "2026-11-15 09:12:00", "2026-11-15 09:12:00", "", "Documents/"},
	}
	asciiOut := RenderTable(&buf, TableConfig{
		Headers:  asciiHeaders,
		Rows:     asciiRows,
		MaxWidth: 80,
		Color:    boolPtr(false),
		ASCII:    boolPtr(true),
	})
	if !strings.Contains(asciiOut, "+") || strings.ContainsAny(asciiOut, "╭╮╰╯│─") {
		t.Fatalf("expected pure ASCII borders in table output:\n%s", asciiOut)
	}
	for _, r := range asciiOut {
		if r > unicode.MaxASCII {
			t.Fatalf("ascii table emitted unexpected non-ASCII rune %U", r)
		}
	}

	// Colored ASCII table where a styled cell truncates right after an ANSI SGR sequence ("\x1b[38;5;75mX…\x1b[0m")
	// must NOT eat the 'm' of the CSI sequence!
	corruptCheck := replaceEllipsisASCIIPreserveWidth("\x1b[38;5;75mX…\x1b[0m|\n\x1b[1m中…\x1b[0m|")
	if strings.Contains(corruptCheck, "75...") || !strings.Contains(corruptCheck, "\x1b[38;5;75m..\x1b[0m|") || !strings.Contains(corruptCheck, "\x1b[1m...\x1b[0m|") {
		t.Fatalf("replaceEllipsisASCIIPreserveWidth corrupted ANSI CSI sequence: %q", corruptCheck)
	}
	coloredASCII := RenderTable(&buf, TableConfig{
		Headers:  asciiHeaders,
		Rows:     asciiRows,
		MaxWidth: 56,
		Color:    boolPtr(true),
		ASCII:    boolPtr(true),
	})
	for _, r := range coloredASCII {
		if r > unicode.MaxASCII {
			t.Fatalf("colored ascii table emitted non-ASCII rune %U", r)
		}
	}
}

func TestMachineOutputAndNotifications(t *testing.T) {
	var jBuf bytes.Buffer
	if err := WriteJSON(&jBuf, map[string]string{"app": "pant"}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(jBuf.Bytes(), &decoded); err != nil || decoded["app"] != "pant" {
		t.Fatalf("invalid JSON output: %s", jBuf.String())
	}

	var tBuf bytes.Buffer
	if err := WriteTSV(&tBuf, [][]string{{"101", "file\tname", "line\nbreak"}}); err != nil {
		t.Fatalf("WriteTSV failed: %v", err)
	}
	if strings.TrimSpace(tBuf.String()) != "101\tfile name\tline break" {
		t.Fatalf("unexpected TSV output: %q", tBuf.String())
	}

	msg := FormatError(&jBuf, "Download failed", errors.New("错误代码: 31045"), "", false, true)
	if !strings.Contains(msg, "建议:") || !strings.Contains(msg, "login") {
		t.Fatalf("expected actionable hint in FormatError output, got:\n%s", msg)
	}
}

func TestMaskUID(t *testing.T) {
	cases := []struct {
		uid  uint64
		want string
	}{
		{0, "0"},
		{7, "***"},
		{42, "4**"},
		{123456, "12***56"},
		{17596386318481, "175****481"},
	}
	for _, tc := range cases {
		if got := MaskUID(tc.uid); got != tc.want {
			t.Fatalf("MaskUID(%d) = %q, want %q", tc.uid, got, tc.want)
		}
	}
}

func TestNoColorRestoreAndHeaderAlign(t *testing.T) {
	defer SetNoColor(false)
	SetNoColor(true)
	if !noColorOverride.Load() {
		t.Fatal("expected noColorOverride=true after SetNoColor(true)")
	}
	SetNoColor(false)
	if noColorOverride.Load() {
		t.Fatal("expected noColorOverride=false after SetNoColor(false)")
	}

	defer SetQuiet(false)
	SetQuiet(true)
	if !IsQuiet() {
		t.Fatal("expected IsQuiet()=true after SetQuiet(true)")
	}
	Statusf("should be suppressed")
	Statusln("should be suppressed")
	SetQuiet(false)

	var buf bytes.Buffer
	ha := AlignRight
	out := RenderTable(&buf, TableConfig{
		Headers:     []string{"Key", "Value"},
		Rows:        [][]string{{"app_id", "266719"}},
		HeaderAlign: &ha,
		MaxWidth:    60,
		Color:       boolPtr(false),
		ASCII:       boolPtr(true),
	})
	if !strings.Contains(out, "Key") || !strings.Contains(out, "266719") {
		t.Fatalf("unexpected RenderTable output with HeaderAlign:\n%s", out)
	}
}

func TestTruncateMiddlePreservesPrefixAndSuffix(t *testing.T) {
	t.Setenv("PANT_ASCII", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	defer SetASCIIOnly(false)
	SetASCIIOnly(false)

	longPath := "/我的资源/2026年度备份/项目归档/核心架构设计说明_final.pdf"
	got := TruncateMiddle(longPath, 26)
	if w := lipgloss.Width(got); w > 26 {
		t.Fatalf("TruncateMiddle width = %d, exceeds 26: %q", w, got)
	}
	if !strings.HasPrefix(got, "/我的资源") || !strings.HasSuffix(got, "final.pdf") || !strings.Contains(got, "…") {
		t.Fatalf("expected middle truncation preserving prefix and filename, got %q", got)
	}
}

func TestFileTypeStylingAndTransferDashboard(t *testing.T) {
	if ClassifyFile("Docs", true) != FileCategoryDir {
		t.Fatal("expected directory classification")
	}
	if ClassifyFile("backup.tar.gz", false) != FileCategoryArchive {
		t.Fatal("expected archive classification")
	}
	if ClassifyFile("movie.mkv", false) != FileCategoryMedia {
		t.Fatal("expected media classification")
	}

	var buf bytes.Buffer
	dirStyled := StyleFileName(&buf, "Projects", true, false, false)
	if dirStyled != "▸ Projects/" {
		t.Fatalf("unexpected unicode dir display: %q", dirStyled)
	}
	asciiDir := StyleFileName(&buf, "Projects", true, false, true)
	if asciiDir != "Projects/" {
		t.Fatalf("unexpected ascii dir display: %q", asciiDir)
	}

	dash := NewTransferDashboard(&buf, "并发下载", 4, true)
	dash.slots["1"] = TransferSlot{
		ID:         "1",
		Name:       "dataset_2026_part1.tar.zst",
		Arrow:      "↓",
		DoneBytes:  512 << 20,
		TotalBytes: 1024 << 20,
		SpeedBytes: 32 << 20,
		ETA:        "16s",
	}
	dash.slots["2"] = TransferSlot{
		ID:         "2",
		Name:       "notes.pdf",
		Arrow:      "↓",
		DoneBytes:  8 << 20,
		TotalBytes: 10 << 20,
		SpeedBytes: 4 << 20,
		ETA:        "1s",
	}
	lines := dash.BuildLines(90)
	if len(lines) != 3 {
		t.Fatalf("expected header + 2 slot lines, got %d:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > 90 {
			t.Fatalf("dashboard line %d width %d > 90: %q", i, w, l)
		}
	}
}

func TestNewRendererColorProfilesAndNilDashboardClearSlot(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("COLORTERM", "")

	var buf bytes.Buffer

	t.Setenv("TERM", "xterm-256color")
	r256 := NewRenderer(&buf, boolPtr(true))
	if r256.ColorProfile() != termenv.ANSI256 {
		t.Fatalf("expected ANSI256 on TERM=xterm-256color without COLORTERM, got %v", r256.ColorProfile())
	}

	t.Setenv("TERM", "xterm")
	r16 := NewRenderer(&buf, boolPtr(true))
	if r16.ColorProfile() != termenv.ANSI {
		t.Fatalf("expected ANSI (16-color) on TERM=xterm without COLORTERM, got %v", r16.ColorProfile())
	}

	t.Setenv("COLORTERM", "truecolor")
	rTrue := NewRenderer(&buf, boolPtr(true))
	if rTrue.ColorProfile() != termenv.TrueColor {
		t.Fatalf("expected TrueColor when COLORTERM=truecolor, got %v", rTrue.ColorProfile())
	}

	// forceColor == nil with FORCE_COLOR / CLICOLOR_FORCE in non-TTY pipe must enable color profile (#24).
	t.Setenv("COLORTERM", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("FORCE_COLOR", "2")
	rForced := NewRenderer(&buf, nil)
	if rForced.ColorProfile() != termenv.ANSI256 {
		t.Fatalf("expected FORCE_COLOR=2 with forceColor=nil to yield ANSI256, got %v", rForced.ColorProfile())
	}

	t.Setenv("FORCE_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	rCliForce := NewRenderer(&buf, nil)
	if rCliForce.ColorProfile() == termenv.Ascii {
		t.Fatalf("expected CLICOLOR_FORCE=1 with forceColor=nil to enable non-Ascii profile, got %v", rCliForce.ColorProfile())
	}

	// TransferDashboard.ClearSlot on nil receiver must still write logLine to os.Stderr (#24).
	oldStderr := os.Stderr
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = pw
	var nilDash *TransferDashboard
	nilDash.ClearSlot("1", "[1] retry log on nil dashboard")
	_ = pw.Close()
	os.Stderr = oldStderr

	var errBuf bytes.Buffer
	_, _ = io.Copy(&errBuf, pr)
	if !strings.Contains(errBuf.String(), "[1] retry log on nil dashboard") {
		t.Fatalf("expected nil TransferDashboard.ClearSlot to write logLine to stderr, got %q", errBuf.String())
	}
}

func TestDashboardNumericSortingAndStrictASCIICompliance(t *testing.T) {
	var buf bytes.Buffer
	dash := NewTransferDashboard(&buf, "BatchDownload", 10, true)
	dash.ascii = true
	dash.color = false

	for _, id := range []string{"10", "2", "1", "3", "4", "5", "6"} {
		dash.slots[id] = TransferSlot{
			ID:         id,
			Name:       "very_long_ascii_filename_for_slot_" + id + ".tar.gz",
			Arrow:      "v",
			DoneBytes:  1024,
			TotalBytes: 2048,
			SpeedBytes: 512,
			ETA:        "2s",
		}
	}

	lines := dash.BuildLines(48)
	if len(lines) != 7 { // header + 5 visible + 1 extra summary
		t.Fatalf("expected 7 lines (header + 5 slots + extra), got %d:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	// Verify numeric ID ordering: [1], [2], [3], [4], [5] (NOT [1], [10], [2]...)
	if !strings.Contains(lines[1], "[1]") || !strings.Contains(lines[2], "[2]") || !strings.Contains(lines[3], "[3]") {
		t.Fatalf("expected numeric slot sorting [1],[2],[3], got:\n%s", strings.Join(lines, "\n"))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > 48 {
			t.Fatalf("dashboard line %d width %d > 48: %q", i, w, l)
		}
		// Header and slot lines have ASCII title/names; ensure zero Unicode box/icon/ellipsis symbols leaked.
		if strings.ContainsAny(l, "⚡…·━╸─") {
			t.Fatalf("dashboard ascii line %d leaked Unicode symbol: %q", i, l)
		}
	}

	prog := FormatTransferProgress(&buf, "1", "v", 0.5, "1MB", "2MB", "500KB", "2s", "2s", false, true, false, "extremely_long_ascii_transfer_archive_name_2026.zip")
	for _, r := range prog {
		if r > unicode.MaxASCII {
			t.Fatalf("FormatTransferProgress ascii=true emitted non-ASCII rune %U in %q", r, prog)
		}
	}
}

func TestRenderTableHeaderlessPreservesAllRowsWithoutBlankLine(t *testing.T) {
	var buf bytes.Buffer

	// 1. Single-row headerless table (e.g. upload/download failure list with 1 item)
	// must render exactly 3 physical lines (top border + 1 row + bottom border), with no phantom blank row.
	oneRowOut := RenderTable(&buf, TableConfig{
		Rows:     [][]string{{"1", "/tmp/failed_upload.bin"}},
		MaxWidth: 80,
		Color:    boolPtr(false),
		ASCII:    boolPtr(false),
	})
	oneRowLines := strings.Split(strings.TrimSuffix(oneRowOut, "\n"), "\n")
	if len(oneRowLines) != 3 {
		t.Fatalf("expected 3 physical lines for 1-row headerless table, got %d:\n%s", len(oneRowLines), oneRowOut)
	}
	if !strings.Contains(oneRowLines[1], "/tmp/failed_upload.bin") {
		t.Fatalf("expected row content on line 1, got %q", oneRowLines[1])
	}

	// 2. Multi-row headerless table (e.g. sumfile 4 rows & meta key-value rows)
	// must preserve the final row (crc32) under both wide and clamped widths.
	sumRows := [][]string{
		{"文件大小", "38300"},
		{"md5", "95e6cff0d03c39f5945491c60a4677a9"},
		{"前256KB切片的md5", "95e6cff0d03c39f5945491c60a4677a9"},
		{"crc32", "2948571234"},
	}
	for _, maxW := range []int{80, 44} {
		multiOut := RenderTable(&buf, TableConfig{
			Rows:     sumRows,
			MaxWidth: maxW,
			Color:    boolPtr(false),
			ASCII:    boolPtr(false),
		})
		multiLines := strings.Split(strings.TrimSuffix(multiOut, "\n"), "\n")
		if len(multiLines) != 6 {
			t.Fatalf("maxWidth=%d: expected 6 physical lines (border + 4 rows + border), got %d:\n%s", maxW, len(multiLines), multiOut)
		}
		if !strings.Contains(multiOut, "crc32") || !strings.Contains(multiOut, "2948571234") {
			t.Fatalf("maxWidth=%d: headerless RenderTable dropped final crc32 row:\n%s", maxW, multiOut)
		}
	}
}

func TestStyleFileSizePreservesPaddingForDirsAndUnpaddedCells(t *testing.T) {
	var buf bytes.Buffer

	for _, colorOn := range []bool{false, true} {
		// Pre-padded '-' (as passed by TUI list pane padLeft("-", 10)) must retain full 10-col width.
		paddedDir := StyleFileSize(&buf, 0, "         -", true, colorOn)
		if colorOn && !strings.Contains(paddedDir, "\x1b[") {
			t.Fatalf("expected ANSI escape sequence when colorOn=true, got %q", paddedDir)
		}
		if !colorOn && strings.Contains(paddedDir, "\x1b[") {
			t.Fatalf("unexpected ANSI escape sequence when colorOn=false: %q", paddedDir)
		}
		if plain := ansi.Strip(paddedDir); plain != "         -" || lipgloss.Width(paddedDir) != 10 {
			t.Fatalf("colorOn=%v: expected 10-col right-aligned '-', got plain=%q width=%d", colorOn, plain, lipgloss.Width(paddedDir))
		}

		// Right-padded numeric formatted string on a directory also preserves column width.
		paddedZeroDir := StyleFileSize(&buf, 0, "        0B", true, colorOn)
		if plain := ansi.Strip(paddedZeroDir); plain != "         -" || lipgloss.Width(paddedZeroDir) != 10 {
			t.Fatalf("colorOn=%v: expected 10-col '-' from padded '0B', got plain=%q width=%d", colorOn, plain, lipgloss.Width(paddedZeroDir))
		}

		// Left-padded '-' preserves trailing spaces.
		leftPaddedDir := StyleFileSize(&buf, 0, "-         ", true, colorOn)
		if plain := ansi.Strip(leftPaddedDir); plain != "-         " || lipgloss.Width(leftPaddedDir) != 10 {
			t.Fatalf("colorOn=%v: expected 10-col left-aligned '-', got plain=%q width=%d", colorOn, plain, lipgloss.Width(leftPaddedDir))
		}

		// Unpadded cell (as passed by `ls` table renderer) must remain single-column '-'.
		unpaddedDir := StyleFileSize(&buf, 0, "0B", true, colorOn)
		if plain := ansi.Strip(unpaddedDir); plain != "-" || lipgloss.Width(unpaddedDir) != 1 {
			t.Fatalf("colorOn=%v: expected unpadded '-', got plain=%q width=%d", colorOn, plain, lipgloss.Width(unpaddedDir))
		}

		// Center-padded cell also preserves full column width.
		centerPaddedDir := StyleFileSize(&buf, 0, "    0B    ", true, colorOn)
		if plain := ansi.Strip(centerPaddedDir); lipgloss.Width(centerPaddedDir) != 10 || strings.TrimSpace(plain) != "-" {
			t.Fatalf("colorOn=%v: expected 10-col center-padded '-', got plain=%q width=%d", colorOn, plain, lipgloss.Width(centerPaddedDir))
		}

		// Regular files across size tiers preserve padded width.
		for _, tc := range []struct {
			bytes int64
			text  string
		}{
			{81, "       81B"},
			{200 << 20, "   200.0MB"},
			{5 << 30, "     5.0GB"},
		} {
			got := StyleFileSize(&buf, tc.bytes, tc.text, false, colorOn)
			if lipgloss.Width(got) != 10 || ansi.Strip(got) != tc.text {
				t.Fatalf("colorOn=%v size=%d: expected %q (width 10), got %q (width %d)", colorOn, tc.bytes, tc.text, ansi.Strip(got), lipgloss.Width(got))
			}
		}
	}
}

func TestTruncateMiddleGraphemeClustersAndPromptSanitize(t *testing.T) {
	cases := []string{
		"❤️_love_❤️_notes_❤️_2026_backup_final.txt",
		"⚠️_warning_⚠️_alert_⚠️_report_2026.md",
		"1️⃣_step_2️⃣_step_3️⃣_guide_2026_final.pdf",
		"🏳️‍🌈_flag_🏳️‍🌈_banner_🏳️‍🌈_archive_2026.zip",
		"👨‍👩‍👧‍👦_family_👨‍👩‍👧‍👦_photo_👨‍👩‍👧‍👦_album.jpg",
		"🇨🇳_flag_🇺🇸_flag_🇯🇵_archive_2026.zip",
		"Cafe\u0301_re\u0301sume\u0301_na\u0308ive_expose\u0301_2026.docx",
	}

	for _, s := range cases {
		for maxW := 6; maxW <= 30; maxW++ {
			for _, ascii := range []bool{false, true} {
				got := TruncateMiddleASCII(s, maxW, ascii)
				if gw := lipgloss.Width(got); gw > maxW {
					t.Fatalf("TruncateMiddleASCII(%q, %d, ascii=%v) width=%d > %d: %q", s, maxW, ascii, gw, maxW, got)
				}
				if strings.Contains(got, "…\u0301") || strings.Contains(got, "...\u0301") ||
					strings.Contains(got, "…\u0308") || strings.Contains(got, "...\u0308") {
					t.Fatalf("TruncateMiddleASCII(%q, %d, ascii=%v) split combining mark onto ellipsis: %q", s, maxW, ascii, got)
				}
				if strings.Contains(got, "\u200d…") || strings.Contains(got, "…\u200d") {
					t.Fatalf("TruncateMiddleASCII(%q, %d, ascii=%v) left dangling ZWJ around ellipsis: %q", s, maxW, ascii, got)
				}
			}
		}
	}

	// PromptContext.Workdir with ANSI escapes must not leak broken CSI fragments.
	var buf bytes.Buffer
	hdr, _ := FormatPrompt(&buf, PromptContext{
		Style:     PromptStyleStarship,
		LoggedIn:  true,
		UserName:  "alice",
		Workdir:   "/\x1b[31msecret\x1b[0m/dir",
		TermWidth: 80,
		Color:     boolPtr(false),
		ASCII:     boolPtr(false),
	})
	if strings.Contains(hdr, "[31m") || strings.Contains(hdr, "\x1b") {
		t.Fatalf("expected ANSI sequences stripped from Workdir in prompt, got %q", hdr)
	}

	// Active boxed inputPrompt must always measure identically in lipgloss and go-runewidth (used by peterh/liner).
	_, activeBoxInp := FormatPrompt(&buf, PromptContext{
		Style:     PromptStyleBoxed,
		LoggedIn:  true,
		UserName:  "alice",
		Workdir:   "/docs",
		TermWidth: 80,
		Color:     boolPtr(false),
	})
	if rw, lw := runewidth.StringWidth(activeBoxInp), lipgloss.Width(activeBoxInp); rw != lw || lw != 4 {
		t.Fatalf("expected boxed inputPrompt %q width 4 in both runewidth (%d) and lipgloss (%d)", activeBoxInp, rw, lw)
	}
	if os.Getenv("RUNEWIDTH_EASTASIAN") != "1" {
		if rw, lw := runewidth.StringWidth("╰─❯ "), lipgloss.Width("╰─❯ "); rw != 4 || lw != 4 {
			t.Fatalf("expected '╰─❯ ' width 4 in both runewidth (%d) and lipgloss (%d)", rw, lw)
		}
	}

	// Explicit RUNEWIDTH_EASTASIAN=1 must activate ASCIIOnly() fallback to avoid 2-cell ambiguous rune drift.
	t.Setenv("PANT_ASCII", "")
	t.Setenv("LANG", "zh_CN.UTF-8")
	t.Setenv("LC_ALL", "")
	SetASCIIOnly(false)
	t.Setenv("RUNEWIDTH_EASTASIAN", "1")
	if !ASCIIOnly() {
		t.Fatal("expected ASCIIOnly() == true when RUNEWIDTH_EASTASIAN=1")
	}
	_, eastAsianInp := FormatPrompt(&buf, PromptContext{
		Style:     PromptStyleBoxed,
		LoggedIn:  true,
		UserName:  "alice",
		Workdir:   "/docs",
		TermWidth: 80,
		Color:     boolPtr(false),
	})
	if eastAsianInp != "+-> " {
		t.Fatalf("expected ASCII boxed prompt '+-> ' under RUNEWIDTH_EASTASIAN=1, got %q", eastAsianInp)
	}
}

func TestSelectOptionNavigationNumberKeysNoReadAheadAndASCII(t *testing.T) {
	defer RestoreOverrides(UIOverrides{})
	RestoreOverrides(UIOverrides{NoColor: true})
	t.Setenv("PANT_ASCII", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")

	items := []string{
		"扫码登录（推荐，支持百度网盘/百度 App）",
		"粘贴 Cookies 登录",
		"输入 BDUSS + STOKEN 登录",
		"账号密码登录",
	}

	// 1. Default Enter selects defaultIdx 0.
	var out bytes.Buffer
	got, err := SelectOption(strings.NewReader("\n"), &out, "选择一种登录方式", items, 0)
	if err != nil || got != 0 {
		t.Fatalf("default Enter: got (%d, %v), want (0, nil)", got, err)
	}
	if !strings.Contains(out.String(), "❯ 1. 扫码登录") || !strings.Contains(out.String(), "✓ 选择一种登录方式: 扫码登录") {
		t.Fatalf("unexpected default menu output:\n%s", out.String())
	}

	// 2. Arrow keys (\x1b[B, \x1b[A, \x1bOA, \x1bOB) + wrap-around + Home/End + CSI sequences.
	for _, tc := range []struct {
		name  string
		input string
		want  int
	}{
		{"down_down_up", "\x1b[B\x1b[B\x1b[A\n", 1},
		{"wrap_up_from_zero", "\x1b[A\n", 3},
		{"wrap_down_to_zero", "\x1bOB\x1bOB\x1bOB\x1bOB\n", 0},
		{"vim_jk", "jjjk\n", 2},
		{"end_then_home_down", "\x1b[F\x1b[H\x1b[B\n", 1},
		{"ignore_csi_pgup_then_num3", "\x1b[5~3\r", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			idx, selErr := SelectOption(strings.NewReader(tc.input), &b, "选择一种登录方式", items, 0)
			if selErr != nil || idx != tc.want {
				t.Fatalf("got (%d, %v), want (%d, nil)", idx, selErr, tc.want)
			}
		})
	}

	// 3. Single-byte reads MUST NOT swallow trailing bytes after CRLF/LF (critical for subsequent Cookie/BDUSS paste).
	stream := strings.NewReader("2\r\nBDUSS=secret_tok; STOKEN=pan_tok\n")
	var menuOut bytes.Buffer
	idx, err := SelectOption(stream, &menuOut, "选择一种登录方式", items, 0)
	if err != nil || idx != 1 {
		t.Fatalf("number key 2 + CRLF: got (%d, %v), want (1, nil)", idx, err)
	}
	rest, err := io.ReadAll(stream)
	if err != nil || string(rest) != "BDUSS=secret_tok; STOKEN=pan_tok\n" {
		t.Fatalf("SelectOption swallowed subsequent stdin bytes! remaining=%q err=%v", string(rest), err)
	}

	// 4. Cancellation via Ctrl+C, Ctrl+D, 'q', double-Esc, and empty EOF.
	for _, cancelInput := range []string{"\x03", "\x04", "q", "\x1b\x1b", ""} {
		var b bytes.Buffer
		_, cancelErr := SelectOption(strings.NewReader(cancelInput), &b, "选择一种登录方式", items, 0)
		if !errors.Is(cancelErr, ErrSelectCanceled) {
			t.Fatalf("input %q: got err=%v, want ErrSelectCanceled", cancelInput, cancelErr)
		}
	}

	// 5. Strict 7-bit ASCII compliance under --ascii + --no-color with ASCII items.
	SetASCIIOnly(true)
	SetNoColor(true)
	var asciiOut bytes.Buffer
	idx, err = SelectOption(strings.NewReader("1\n"), &asciiOut, "Select login", []string{"QR", "Cookie"}, 0)
	if err != nil || idx != 0 {
		t.Fatalf("ASCII SelectOption failed: (%d, %v)", idx, err)
	}
	for _, r := range asciiOut.String() {
		if r > unicode.MaxASCII {
			t.Fatalf("ASCII SelectOption emitted non-ASCII rune %U in:\n%s", r, asciiOut.String())
		}
	}
}
