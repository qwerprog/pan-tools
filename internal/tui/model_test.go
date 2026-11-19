package tui

import (
	"path"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/rivo/uniseg"
)

type fakeCloudBackend struct {
	tree        map[string]baidupcs.FileDirectoryList
	mkdirPath   string
	renamedFrom string
	renamedTo   string
	renameCalls int
	uploaded    []string
	removed     []string
	workdir     string
}

func newFakeCloudBackend() *fakeCloudBackend {
	return &fakeCloudBackend{
		tree: map[string]baidupcs.FileDirectoryList{
			"/": {
				{Filename: "readme.md", Path: "/readme.md", Size: 2048, Mtime: 1700000000},
				{Filename: "Movies", Path: "/Movies", Isdir: true, Mtime: 1700000100},
				{Filename: "Archive.zip", Path: "/Archive.zip", Size: 5 << 20, Mtime: 1700000200},
			},
			"/Movies": {
				{Filename: "clip.mp4", Path: "/Movies/clip.mp4", Size: 200 << 20, Mtime: 1700000300},
			},
		},
	}
}

func (f *fakeCloudBackend) ListDir(dir string, _ *baidupcs.OrderOptions, _ bool) (baidupcs.FileDirectoryList, error) {
	return f.tree[path.Clean(dir)], nil
}

func (f *fakeCloudBackend) Mkdir(fullPath string) error {
	f.mkdirPath = fullPath
	parent := path.Dir(fullPath)
	f.tree[parent] = append(f.tree[parent], &baidupcs.FileDirectory{
		Filename: path.Base(fullPath),
		Path:     fullPath,
		Isdir:    true,
	})
	return nil
}

func (f *fakeCloudBackend) Rename(from, to string) error {
	f.renameCalls++
	f.renamedFrom = from
	f.renamedTo = to
	return nil
}

func (f *fakeCloudBackend) Remove(paths ...string) error {
	f.removed = append([]string(nil), paths...)
	return nil
}

func (f *fakeCloudBackend) Quota() (int64, int64, error) {
	return 2 << 40, 1 << 40, nil
}

func (f *fakeCloudBackend) Membership() (*baidupcs.MembershipInfo, error) {
	return &baidupcs.MembershipInfo{Label: "SVIP", IsVIP: true, IsSVIP: true}, nil
}

func (f *fakeCloudBackend) Download([]string) error { return nil }
func (f *fakeCloudBackend) Upload(paths []string, _ string) error {
	f.uploaded = append([]string(nil), paths...)
	return nil
}
func (f *fakeCloudBackend) SaveWorkdir(dir string) { f.workdir = dir }
func (f *fakeCloudBackend) UserSummary() (string, uint64, string, bool) {
	return "tester", 12345678, "/tmp/Downloads", true
}

func sendKey(m *Model, k tea.KeyMsg) *Model {
	next, cmd := m.Update(k)
	m = next.(*Model)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, sub := range batch {
					if sub != nil {
						next2, _ := m.Update(sub())
						m = next2.(*Model)
					}
				}
			} else {
				next2, _ := m.Update(msg)
				m = next2.(*Model)
			}
		}
	}
	return m
}

func TestTUIExplorerNavigationFilterSelectAndActions(t *testing.T) {
	backend := newFakeCloudBackend()
	m := NewModel(backend, "/")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Update(dirLoadedMsg{dir: "/", files: backend.tree["/"]})
	m.Update(metaLoadedMsg{quotaTotal: 2 << 40, quotaUsed: 1 << 40, hasQuota: true, vipLabel: "SVIP", isVIP: true, isSVIP: true})

	// 目录应排在文件之前
	if cur := m.CurrentItem(); cur == nil || cur.Filename != "Movies" {
		t.Fatalf("expected directory 'Movies' sorted first, got %+v", cur)
	}

	// 回车进入子目录，再按 Backspace 返回根目录并恢复光标位置
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.cwd != "/Movies" || len(m.visible) != 1 {
		t.Fatalf("expected cwd=/Movies with 1 item, got cwd=%q len=%d", m.cwd, len(m.visible))
	}
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.cwd != "/" {
		t.Fatalf("expected return to '/', got %q", m.cwd)
	}

	// 模糊过滤 '/'
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zip")})
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.visible) != 1 || m.visible[0].Filename != "Archive.zip" {
		t.Fatalf("expected filter 'zip' to match Archive.zip, got %+v", m.visible)
	}
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.visible) != 3 {
		t.Fatalf("expected Esc to clear filter back to 3 items, got %d", len(m.visible))
	}

	// Space 多选 + x 删除确认
	m = sendKey(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if len(m.selected) != 1 {
		t.Fatalf("expected 1 selected item after Space, got %d", len(m.selected))
	}
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if len(backend.removed) != 1 || backend.removed[0] != "/Movies" {
		t.Fatalf("expected '/Movies' removed, got %#v", backend.removed)
	}

	// n 新建目录
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Docs")})
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if backend.mkdirPath != "/Docs" {
		t.Fatalf("expected Mkdir('/Docs'), got %q", backend.mkdirPath)
	}

	// 渲染双栏宽屏与窄屏及帮助浮层均不越界或 panic
	wideView := m.View()
	if !strings.Contains(wideView, "pan-tools Explorer") || !strings.Contains(wideView, "详情预览") {
		t.Fatalf("unexpected wide TUI view:\n%s", wideView)
	}

	m.Update(tea.WindowSizeMsg{Width: 78, Height: 24})
	narrowView := m.View()
	if strings.Contains(narrowView, "详情预览") {
		t.Fatal("expected inspector pane hidden on narrow <94 col terminal")
	}

	m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	browseView := m.View()

	m = sendKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	helpView := m.View()
	if !strings.Contains(helpView, "快捷键指南") {
		t.Fatalf("expected help modal in view:\n%s", helpView)
	}
	if lipgloss.Height(helpView) != lipgloss.Height(browseView) {
		t.Fatalf("help modal height %d != browse view height %d (causes footer jump)", lipgloss.Height(helpView), lipgloss.Height(browseView))
	}

	// 校验帮助卡片上中下边框起始列完全对齐（防止首行偏移或内容折行错位）
	borderCols := -1
	borderLines := 0
	for _, line := range strings.Split(helpView, "\n") {
		plain := ansi.Strip(line)
		for idx, ch := range []string{"╭", "│", "╰"} {
			_ = idx
			if pos := strings.Index(plain, ch); pos >= 0 {
				if borderCols == -1 {
					borderCols = pos
				} else if pos != borderCols {
					t.Fatalf("help card border misaligned: expected byte offset %d, got %d on line %q", borderCols, pos, plain)
				}
				borderLines++
				break
			}
		}
	}
	if borderLines < 14 {
		t.Fatalf("expected >=14 aligned border lines in help card, got %d", borderLines)
	}

	// 验证标准 24 行终端下帮助浮层同样不溢出总高度、不丢失底栏快捷键
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	help24 := m.View()
	if lipgloss.Height(help24) > 24 {
		t.Fatalf("24-row terminal help view overflowed: got %d lines", lipgloss.Height(help24))
	}
	if !strings.Contains(help24, "移动") || !strings.Contains(help24, "退出") {
		t.Fatalf("bottom keybar clipped on 24-row terminal:\n%s", help24)
	}
}

func TestTUIOutOfOrderReqIDLoadingLockAndCrossDirSelection(t *testing.T) {
	backend := newFakeCloudBackend()
	m := NewModel(backend, "/")
	_ = m.Init() // reqID == 1
	req1 := m.dirReqID
	m.Update(dirLoadedMsg{reqID: req1, dir: "/", files: backend.tree["/"]})

	// Select an item in "/", then enter "/Movies" (#12: cross-dir selection must be cleared).
	m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if len(m.selected) != 1 {
		t.Fatalf("expected 1 selected item in '/', got %d", len(m.selected))
	}
	// Move back to Movies (index 0) and press Enter to start loading /Movies
	m.cursor = 0
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.loading {
		t.Fatal("expected m.loading == true after Enter on directory")
	}
	if len(m.selected) != 0 {
		t.Fatalf("expected cross-directory selection cleared on directory change, got %#v", m.selected)
	}
	// While m.loading == true, stale item actions (x delete, Space select, R rename) must be ignored (#11).
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.mode == modeConfirmDelete {
		t.Fatal("must not enter modeConfirmDelete while directory is still loading")
	}

	// Out-of-order stale response with old reqID must be discarded (#11).
	req2 := m.dirReqID
	m.Update(dirLoadedMsg{reqID: req1, dir: "/stale", files: baidupcs.FileDirectoryList{{Filename: "ghost.txt", Path: "/stale/ghost.txt"}}})
	if !m.loading || m.cwd != "/Movies" {
		t.Fatalf("stale reqID message should have been ignored, got loading=%v cwd=%q", m.loading, m.cwd)
	}

	// Matching req2 response succeeds.
	m.Update(dirLoadedMsg{reqID: req2, dir: "/Movies", files: backend.tree["/Movies"]})
	if m.loading || len(m.visible) != 1 || m.visible[0].Filename != "clip.mp4" {
		t.Fatalf("expected /Movies loaded with clip.mp4, got loading=%v visible=%+v", m.loading, m.visible)
	}
}

func TestTUIRenameSnapshotNoOpPruneInputSanitizeAndNarrowShortViews(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	backend := newFakeCloudBackend()
	m := NewModel(backend, "/Movies")
	m.Update(dirLoadedMsg{dir: "/Movies", files: backend.tree["/Movies"]})

	// Select clip.mp4, press R, then press Enter without changing name => no-op, 0 backend Rename calls (#12).
	m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if backend.renameCalls != 0 {
		t.Fatalf("expected 0 Rename calls when name unchanged, got %d", backend.renameCalls)
	}
	if _, stillSelected := m.selected["/Movies/clip.mp4"]; !stillSelected {
		t.Fatal("unchanged rename no-op should keep selection intact")
	}

	// Press R again, mutate cursor while in modeInputRename to verify snapshotted target is used,
	// test Ctrl+H backspace + pasted newline stripping (#12, #13), and verify old selection is pruned!
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m.cursor = 99 // simulate cursor drift
	for i := 0; i < len("clip.mp4"); i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("renamed\n\r.mp4")})
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if backend.renamedFrom != "/Movies/clip.mp4" || backend.renamedTo != "/Movies/renamed.mp4" {
		t.Fatalf("unexpected rename (%q -> %q)", backend.renamedFrom, backend.renamedTo)
	}
	if _, leaked := m.selected["/Movies/clip.mp4"]; leaked {
		t.Fatal("expected old path pruned from m.selected after rename")
	}

	// Upload input expands quoted ~/... path (#13): test via handleInputKey.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("'~/upload_test.bin'\n")})
	if m.inputValue != "'~/upload_test.bin'" {
		t.Fatalf("expected newline stripped from pasted inputValue, got %q", m.inputValue)
	}
	_, execCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if execCmd == nil {
		t.Fatal("expected tea.Exec cmd from upload Enter")
	}

	// Narrow terminal (<64 cols AND <24 cols) must NOT force min width and wrap lines (#14),
	// and short terminal (<=21 rows) help card must NOT exceed browse view height (#14).
	for _, dims := range [][2]int{{18, 12}, {20, 14}, {44, 18}, {52, 21}} {
		w, h := dims[0], dims[1]
		m.mode = modeBrowse
		m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		browseOut := m.View()
		for i, line := range strings.Split(browseOut, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Fatalf("narrow (%dx%d) browse line %d width %d > %d: %q", w, h, i, lw, w, line)
			}
		}

		m.mode = modeHelp
		helpOut := m.View()
		if lipgloss.Height(helpOut) != lipgloss.Height(browseOut) {
			t.Fatalf("short (%dx%d) help height %d != browse height %d", w, h, lipgloss.Height(helpOut), lipgloss.Height(browseOut))
		}
		if lipgloss.Height(helpOut) > h {
			t.Fatalf("short (%dx%d) help height %d overflowed terminal height %d", w, h, lipgloss.Height(helpOut), h)
		}
		for i, line := range strings.Split(helpOut, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Fatalf("narrow (%dx%d) help line %d width %d > %d: %q", w, h, i, lw, w, line)
			}
		}
	}

	// --ascii mode in TUI (including sort toggle 's' toast) must not emit ◈ / ❯ / █ / Σ / … / ↑ / ↓ (#22).
	ui.SetASCIIOnly(true)
	defer ui.SetASCIIOnly(false)
	m.mode = modeFilter
	m.filterQuery = "mp4"
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 26})
	asciiView := m.View()
	if strings.ContainsAny(asciiView, "◈❯█Σ…╭╮╰╯│─↑↓") {
		t.Fatalf("TUI --ascii filter view leaked Unicode UI symbol:\n%s", asciiView)
	}
	m.mode = modeBrowse
	m.filterQuery = ""
	m = sendKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	asciiSortView := m.View()
	if strings.ContainsAny(asciiSortView, "◈❯█Σ…╭╮╰╯│─↑↓") {
		t.Fatalf("TUI --ascii sort toggle leaked Unicode UI symbol:\n%s", asciiSortView)
	}
}

func TestTUIListPaneColumnAlignmentAcrossDirSelectedAndFileRows(t *testing.T) {
	snap := ui.SnapshotOverrides()
	defer ui.RestoreOverrides(snap)

	const sampleMtime = 1749526938 // 2025-06-10 11:42:18 CST
	files := baidupcs.FileDirectoryList{
		{Filename: ".idea", Path: "/123/.idea", Isdir: true, Mtime: sampleMtime},
		{Filename: "lib", Path: "/123/lib", Isdir: true, Mtime: sampleMtime},
		{Filename: "src", Path: "/123/src", Isdir: true, Mtime: sampleMtime},
		{Filename: "target", Path: "/123/target", Isdir: true, Size: 4096, Mtime: sampleMtime},
		{Filename: "架构设计归档_超长中文目录名称_2026年度备份测试目录", Path: "/123/架构设计归档_超长中文目录名称_2026年度备份测试目录", Isdir: true, Mtime: sampleMtime},
		{Filename: "❤️_love_❤️_notes_❤️_2026_backup_final_dir", Path: "/123/❤️_love_❤️_notes_❤️_2026_backup_final_dir", Isdir: true, Size: 4096, Mtime: sampleMtime},
		{Filename: "demo03.iml", Path: "/123/demo03.iml", Size: 81, Mtime: sampleMtime},
		{Filename: "pom.xml", Path: "/123/pom.xml", Size: 792, Mtime: sampleMtime},
		{Filename: "dataset_release_2026_final_bundle_x86_64.tar.zst", Path: "/123/dataset_release_2026_final_bundle_x86_64.tar.zst", Size: 5 << 30, Mtime: sampleMtime},
		{Filename: "1️⃣_step_2️⃣_step_3️⃣_guide_2026_release_notes_final.pdf", Path: "/123/1️⃣_step_2️⃣_step_3️⃣_guide_2026_release_notes_final.pdf", Size: 2048, Mtime: sampleMtime},
		{Filename: "Cafe\u0301_re\u0301sume\u0301_na\u0308ive_expose\u0301_2026_final_report.docx", Path: "/123/Cafe\u0301_re\u0301sume\u0301_na\u0308ive_expose\u0301_2026_final_report.docx", Size: 4096, Mtime: sampleMtime},
		{Filename: "bidi_\u202eevil_override\u202c_invalid\xffutf8.bin", Path: "/123/bidi_\u202eevil_override\u202c_invalid\xffutf8.bin", Size: 0, Mtime: sampleMtime},
		{Filename: "bad\tname\nctrl.txt", Path: "/123/bad\tname\nctrl.txt", Size: 1536, Mtime: sampleMtime},
	}

	backend := &fakeCloudBackend{
		tree: map[string]baidupcs.FileDirectoryList{
			"/123": files,
		},
	}

	for _, colorOn := range []bool{true, false} {
		for _, asciiOn := range []bool{false, true} {
			for _, width := range []int{194, 192, 120, 98, 93, 80, 62, 50, 38} {
				for _, cursorIdx := range []int{2, 6} {
					if colorOn {
						t.Setenv("TERM", "xterm-256color")
						t.Setenv("NO_COLOR", "")
						t.Setenv("CLICOLOR", "")
						t.Setenv("FORCE_COLOR", "3")
						ui.SetNoColor(false)
					} else {
						t.Setenv("FORCE_COLOR", "")
						t.Setenv("CLICOLOR_FORCE", "")
						ui.SetNoColor(true)
					}
					t.Setenv("PANT_ASCII", "")
					t.Setenv("RUNEWIDTH_EASTASIAN", "")
					t.Setenv("LANG", "zh_CN.UTF-8")
					t.Setenv("LC_ALL", "")
					ui.SetASCIIOnly(asciiOn)

					m := NewModel(backend, "/123")
					m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
					m.Update(dirLoadedMsg{dir: "/123", files: files})
					m.Update(metaLoadedMsg{quotaTotal: 15 << 40, quotaUsed: 12 << 40, hasQuota: true, vipLabel: "SVIP 7", isVIP: true, isSVIP: true})
					m.cwd = "/123/sub\tdir"
					m.statusMsg = "删除失败: errno=-9\n文件不存在\t请刷新"
					m.statusErr = true

					// Mark one non-cursor dir, cursor dir (when cursorIdx==2), and one file as multi-selected.
					m.selected["/123/lib"] = m.visible[1]
					m.selected["/123/src"] = m.visible[2]
					m.selected["/123/pom.xml"] = m.visible[7]
					m.cursor = cursorIdx

					out := m.View()
					if lipgloss.Height(out) > 30 {
						t.Fatalf("color=%v ascii=%v width=%d: multi-line statusMsg caused View() height %d > 30:\n%s",
							colorOn, asciiOn, width, lipgloss.Height(out), out)
					}
					if strings.ContainsAny(out, "\t\r\u202e\u202c") {
						t.Fatalf("color=%v ascii=%v width=%d: View() leaked raw tab/CR/bidi control char", colorOn, asciiOn, width)
					}
					if colorOn && !strings.Contains(out, "\x1b[") {
						t.Fatalf("colorOn=true ascii=%v width=%d: expected ANSI escape sequences in View()", asciiOn, width)
					}
					if !colorOn && strings.Contains(out, "\x1b[") {
						t.Fatalf("colorOn=false ascii=%v width=%d: unexpected ANSI escape sequence in View()", asciiOn, width)
					}

					rawLines := strings.Split(out, "\n")
					if len(rawLines) < 3+1+len(files) {
						t.Fatalf("unexpected line count %d for width=%d:\n%s", len(rawLines), width, out)
					}

					// Line 2 is the top border of leftBox (and optional rightBox).
					topBorder := ansi.Strip(rawLines[2])
					leftBorderWidth := findLeftBoxWidth(t, topBorder, asciiOn)

					hdrLeft := sliceDisplayRange(ansi.Strip(rawLines[3]), 0, leftBorderWidth)
					hdrSizeStart, hdrSizeEnd := findDisplaySpan(hdrLeft, "大小")
					if hdrSizeStart < 0 {
						t.Fatalf("color=%v ascii=%v width=%d: missing '大小' in header %q", colorOn, asciiOn, width, hdrLeft)
					}
					sizeFieldStart := hdrSizeEnd - 10

					hasDate := strings.Contains(hdrLeft, "修改时间")
					hdrDateStart, _ := findDisplaySpan(hdrLeft, "修改时间")
					if hasDate {
						if hdrDateStart != hdrSizeEnd+2 {
							t.Fatalf("color=%v ascii=%v width=%d: header gap between '大小' end (%d) and '修改时间' start (%d) != 2 in %q",
								colorOn, asciiOn, width, hdrSizeEnd, hdrDateStart, hdrLeft)
						}
						if hdrDateStart+16 != leftBorderWidth-2 {
							t.Fatalf("color=%v ascii=%v width=%d: header date end (%d) != right border - 1 (%d) in %q",
								colorOn, asciiOn, width, hdrDateStart+16, leftBorderWidth-2, hdrLeft)
						}
					} else if hdrSizeEnd != leftBorderWidth-2 {
						t.Fatalf("color=%v ascii=%v width=%d: narrow header size end (%d) != right border - 1 (%d) in %q",
							colorOn, asciiOn, width, hdrSizeEnd, leftBorderWidth-2, hdrLeft)
					}

					wantBorderChar := "│"
					if asciiOn {
						wantBorderChar = "|"
					}

					for rowIdx := 0; rowIdx < len(files); rowIdx++ {
						plainLine := ansi.Strip(rawLines[4+rowIdx])
						leftRow := sliceDisplayRange(plainLine, 0, leftBorderWidth)
						if lipgloss.Width(leftRow) != leftBorderWidth {
							t.Fatalf("color=%v ascii=%v width=%d cur=%d row=%d: leftRow width=%d != %d: %q",
								colorOn, asciiOn, width, cursorIdx, rowIdx, lipgloss.Width(leftRow), leftBorderWidth, leftRow)
						}
						if sliceDisplayRange(leftRow, 0, 1) != wantBorderChar || sliceDisplayRange(leftRow, leftBorderWidth-1, leftBorderWidth) != wantBorderChar {
							t.Fatalf("color=%v ascii=%v width=%d cur=%d row=%d: border misaligned in %q",
								colorOn, asciiOn, width, cursorIdx, rowIdx, leftRow)
						}

						// Separator column immediately before 10-col size field must be space.
						if sep := sliceDisplayRange(leftRow, sizeFieldStart-1, sizeFieldStart); sep != " " {
							t.Fatalf("color=%v ascii=%v width=%d cur=%d row=%d: expected space at col %d before size field, got %q in %q",
								colorOn, asciiOn, width, cursorIdx, rowIdx, sizeFieldStart-1, sep, leftRow)
						}

						sizeSlice := sliceDisplayRange(leftRow, sizeFieldStart, hdrSizeEnd)
						trimmedSize := strings.TrimLeft(sizeSlice, " ")
						if trimmedSize == "" || strings.HasSuffix(sizeSlice, " ") {
							t.Fatalf("color=%v ascii=%v width=%d cur=%d row=%d (%s): size cell %q not right-aligned to col %d in %q",
								colorOn, asciiOn, width, cursorIdx, rowIdx, m.visible[rowIdx].Filename, sizeSlice, hdrSizeEnd, leftRow)
						}
						if m.visible[rowIdx].Isdir && trimmedSize != "-" {
							t.Fatalf("color=%v ascii=%v width=%d cur=%d row=%d (%s): expected dir size '-', got %q in %q",
								colorOn, asciiOn, width, cursorIdx, rowIdx, m.visible[rowIdx].Filename, trimmedSize, leftRow)
						}
						if !m.visible[rowIdx].Isdir && !strings.HasSuffix(trimmedSize, "B") {
							t.Fatalf("color=%v ascii=%v width=%d cur=%d row=%d (%s): expected file size ending in 'B', got %q in %q",
								colorOn, asciiOn, width, cursorIdx, rowIdx, m.visible[rowIdx].Filename, trimmedSize, leftRow)
						}

						if hasDate {
							if gap := sliceDisplayRange(leftRow, hdrSizeEnd, hdrDateStart); gap != "  " {
								t.Fatalf("color=%v ascii=%v width=%d cur=%d row=%d: expected 2-space gap [%d,%d), got %q in %q",
									colorOn, asciiOn, width, cursorIdx, rowIdx, hdrSizeEnd, hdrDateStart, gap, leftRow)
							}
							dateSlice := sliceDisplayRange(leftRow, hdrDateStart, hdrDateStart+16)
							if dateSlice != "2025-06-10 11:42" {
								t.Fatalf("color=%v ascii=%v width=%d cur=%d row=%d (%s): date misaligned at [%d,%d), got %q in %q",
									colorOn, asciiOn, width, cursorIdx, rowIdx, m.visible[rowIdx].Filename, hdrDateStart, hdrDateStart+16, dateSlice, leftRow)
							}
							if padAfter := sliceDisplayRange(leftRow, hdrDateStart+16, leftBorderWidth-1); padAfter != " " {
								t.Fatalf("color=%v ascii=%v width=%d cur=%d row=%d: expected 1 trailing padding space before border, got %q in %q",
									colorOn, asciiOn, width, cursorIdx, rowIdx, padAfter, leftRow)
							}
						}
					}
				}
			}
		}
	}
}

func findLeftBoxWidth(t *testing.T, plainTopBorder string, asciiOn bool) int {
	t.Helper()
	col := 0
	for i, r := range plainTopBorder {
		rw := lipgloss.Width(string(r))
		if !asciiOn && r == '╮' {
			return col + rw
		}
		if asciiOn && i > 0 && r == '+' {
			return col + rw
		}
		col += rw
	}
	t.Fatalf("could not locate right corner of leftBox in top border %q", plainTopBorder)
	return 0
}

func findDisplaySpan(plain, sub string) (int, int) {
	idx := strings.Index(plain, sub)
	if idx < 0 {
		return -1, -1
	}
	start := lipgloss.Width(plain[:idx])
	return start, start + lipgloss.Width(sub)
}

func sliceDisplayRange(plain string, startCol, endCol int) string {
	var sb strings.Builder
	col := 0
	state := -1
	rest := plain
	for len(rest) > 0 {
		var cluster string
		var cw int
		cluster, rest, cw, state = uniseg.FirstGraphemeClusterInString(rest, state)
		if col+cw > startCol && col < endCol {
			sb.WriteString(cluster)
		}
		col += cw
		if col >= endCol {
			break
		}
	}
	return sb.String()
}
