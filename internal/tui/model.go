package tui

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
)

type uiMode int

const (
	modeBrowse uiMode = iota
	modeFilter
	modeInputMkdir
	modeInputRename
	modeInputUpload
	modeConfirmDelete
	modeHelp
)

type sortPreset struct {
	label      string
	asciiLabel string
	opt        *baidupcs.OrderOptions
}

var sortPresets = []sortPreset{
	{label: "名称 ↑", asciiLabel: "名称 ^", opt: &baidupcs.OrderOptions{By: baidupcs.OrderByName, Order: baidupcs.OrderAsc}},
	{label: "时间 ↓", asciiLabel: "时间 v", opt: &baidupcs.OrderOptions{By: baidupcs.OrderByTime, Order: baidupcs.OrderDesc}},
	{label: "大小 ↓", asciiLabel: "大小 v", opt: &baidupcs.OrderOptions{By: baidupcs.OrderBySize, Order: baidupcs.OrderDesc}},
}

func sortPresetLabel(idx int, ascii bool) string {
	if idx < 0 || idx >= len(sortPresets) {
		idx = 0
	}
	if ascii {
		return sortPresets[idx].asciiLabel
	}
	return sortPresets[idx].label
}

type dirLoadedMsg struct {
	reqID uint64
	dir   string
	files baidupcs.FileDirectoryList
	err   error
}

type metaLoadedMsg struct {
	quotaTotal int64
	quotaUsed  int64
	hasQuota   bool
	vipLabel   string
	isVIP      bool
	isSVIP     bool
}

type opDoneMsg struct {
	toast       string
	isErr       bool
	reloadDir   bool
	clearSelect bool
	pruneSelect string
}

// Model 是 pan-tools 全屏网盘浏览器的 Bubble Tea 状态机。
type Model struct {
	backend CloudBackend

	width  int
	height int

	cwd         string
	sortIdx     int
	loading     bool
	dirInFlight bool
	dirReqID    uint64
	allFiles    baidupcs.FileDirectoryList
	visible     baidupcs.FileDirectoryList
	cursor      int
	scrollTop   int
	selected    map[string]*baidupcs.FileDirectory
	lastDirPos  map[string]int

	mode         uiMode
	filterQuery  string
	inputValue   string
	renameSource string

	userName   string
	uid        uint64
	saveDir    string
	vipLabel   string
	isVIP      bool
	isSVIP     bool
	quotaTotal int64
	quotaUsed  int64
	hasQuota   bool

	statusMsg string
	statusErr bool
}

// NewModel 初始化网盘交互浏览器模型。
func NewModel(backend CloudBackend, startDir string) *Model {
	if startDir == "" {
		startDir = "/"
	}
	userName, uid, saveDir, _ := backend.UserSummary()
	return &Model{
		backend:    backend,
		width:      100,
		height:     28,
		cwd:        path.Clean(startDir),
		loading:    true,
		selected:   make(map[string]*baidupcs.FileDirectory),
		lastDirPos: make(map[string]int),
		userName:   userName,
		uid:        uid,
		saveDir:    saveDir,
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.loadDirCmd(m.cwd, false), m.loadMetaCmd())
}

func (m *Model) loadDirCmd(dir string, force bool) tea.Cmd {
	m.dirReqID++
	m.dirInFlight = true
	m.loading = true
	reqID := m.dirReqID
	opt := sortPresets[m.sortIdx].opt
	cleanDir := path.Clean(dir)
	return func() tea.Msg {
		files, err := m.backend.ListDir(cleanDir, opt, force)
		return dirLoadedMsg{reqID: reqID, dir: cleanDir, files: files, err: err}
	}
}

func (m *Model) loadMetaCmd() tea.Cmd {
	return func() tea.Msg {
		var out metaLoadedMsg
		if total, used, err := m.backend.Quota(); err == nil && total > 0 {
			out.quotaTotal = total
			out.quotaUsed = used
			out.hasQuota = true
		}
		if mi, err := m.backend.Membership(); err == nil && mi != nil {
			out.vipLabel = mi.Label
			out.isVIP = mi.IsVIP
			out.isSVIP = mi.IsSVIP
		}
		return out
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		m.clampCursor()
		return m, nil

	case dirLoadedMsg:
		if msg.reqID != m.dirReqID {
			return m, nil
		}
		m.dirInFlight = false
		m.loading = false
		if msg.err != nil {
			m.statusMsg = msg.err.Error()
			m.statusErr = true
			return m, nil
		}
		cleanDir := path.Clean(msg.dir)
		if cleanDir != path.Clean(m.cwd) {
			m.selected = make(map[string]*baidupcs.FileDirectory)
		}
		m.cwd = cleanDir
		m.allFiles = sortDirsFirst(msg.files)
		m.applyFilter()
		if saved, ok := m.lastDirPos[m.cwd]; ok && saved < len(m.visible) {
			m.cursor = saved
		} else {
			m.cursor = 0
		}
		m.clampCursor()
		return m, nil

	case metaLoadedMsg:
		if msg.hasQuota {
			m.quotaTotal = msg.quotaTotal
			m.quotaUsed = msg.quotaUsed
			m.hasQuota = true
		}
		if msg.vipLabel != "" {
			m.vipLabel = msg.vipLabel
			m.isVIP = msg.isVIP
			m.isSVIP = msg.isSVIP
		}
		return m, nil

	case opDoneMsg:
		m.loading = m.dirInFlight
		m.statusMsg = msg.toast
		m.statusErr = msg.isErr
		if msg.clearSelect {
			m.selected = make(map[string]*baidupcs.FileDirectory)
		} else if msg.pruneSelect != "" {
			delete(m.selected, msg.pruneSelect)
		}
		if msg.reloadDir {
			return m, tea.Batch(m.loadDirCmd(m.cwd, true), m.loadMetaCmd())
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case modeHelp:
		switch msg.String() {
		case "?", "esc", "q", "enter":
			m.mode = modeBrowse
		}
		return m, nil

	case modeFilter:
		return m.handleFilterKey(msg)

	case modeInputMkdir, modeInputRename, modeInputUpload:
		return m.handleInputKey(msg)

	case modeConfirmDelete:
		switch strings.ToLower(msg.String()) {
		case "y", "enter":
			targets := m.actionPaths()
			m.mode = modeBrowse
			if len(targets) == 0 {
				return m, nil
			}
			m.loading = true
			return m, func() tea.Msg {
				if err := m.backend.Remove(targets...); err != nil {
					return opDoneMsg{toast: "删除失败: " + err.Error(), isErr: true}
				}
				return opDoneMsg{
					toast:       fmt.Sprintf("已将 %d 项移入回收站", len(targets)),
					reloadDir:   true,
					clearSelect: true,
				}
			}
		default:
			m.mode = modeBrowse
			m.statusMsg = "已取消删除"
			m.statusErr = false
			return m, nil
		}

	default:
		return m.handleBrowseKey(msg)
	}
}

func sanitizeInputRunes(runes []rune) string {
	if len(runes) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, r := range runes {
		if r == '\r' || r == '\n' {
			continue
		}
		if r == '\t' {
			sb.WriteByte(' ')
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func (m *Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.filterQuery = ""
		m.mode = modeBrowse
		m.applyFilter()
	case tea.KeyEnter:
		m.mode = modeBrowse
	case tea.KeyBackspace, tea.KeyDelete, tea.KeyCtrlH:
		runes := []rune(m.filterQuery)
		if len(runes) > 0 {
			m.filterQuery = string(runes[:len(runes)-1])
			m.applyFilter()
		}
	case tea.KeySpace:
		if len(msg.Runes) > 0 {
			m.filterQuery += sanitizeInputRunes(msg.Runes)
		} else {
			m.filterQuery += " "
		}
		m.applyFilter()
	case tea.KeyRunes:
		if clean := sanitizeInputRunes(msg.Runes); clean != "" {
			m.filterQuery += clean
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeBrowse
		m.inputValue = ""
		m.renameSource = ""
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete, tea.KeyCtrlH:
		runes := []rune(m.inputValue)
		if len(runes) > 0 {
			m.inputValue = string(runes[:len(runes)-1])
		}
		return m, nil
	case tea.KeyEnter:
		val := strings.TrimSpace(m.inputValue)
		curMode := m.mode
		oldRenamePath := m.renameSource
		m.mode = modeBrowse
		m.inputValue = ""
		m.renameSource = ""
		if val == "" {
			return m, nil
		}
		switch curMode {
		case modeInputMkdir:
			target := path.Join(m.cwd, val)
			m.loading = true
			return m, func() tea.Msg {
				if err := m.backend.Mkdir(target); err != nil {
					return opDoneMsg{toast: "创建目录失败: " + err.Error(), isErr: true}
				}
				return opDoneMsg{toast: "已创建目录 " + target, reloadDir: true}
			}
		case modeInputRename:
			if oldRenamePath == "" {
				if cur := m.CurrentItem(); cur != nil {
					oldRenamePath = cur.Path
				}
			}
			if oldRenamePath == "" {
				return m, nil
			}
			newPath := path.Clean(path.Join(path.Dir(oldRenamePath), val))
			if strings.HasPrefix(val, "/") {
				newPath = path.Clean(val)
			}
			if newPath == path.Clean(oldRenamePath) {
				m.statusMsg = "名称未变更"
				m.statusErr = false
				return m, nil
			}
			m.loading = true
			return m, func() tea.Msg {
				if err := m.backend.Rename(oldRenamePath, newPath); err != nil {
					return opDoneMsg{toast: "重命名失败: " + err.Error(), isErr: true}
				}
				return opDoneMsg{
					toast:       fmt.Sprintf("已重命名为 %s", path.Base(newPath)),
					reloadDir:   true,
					pruneSelect: oldRenamePath,
				}
			}
		case modeInputUpload:
			expanded := pcsutil.ExpandLocalPath(val)
			if expanded == "" {
				return m, nil
			}
			saveDir := m.cwd
			task := &inProcessTask{
				fn: func() error {
					return m.backend.Upload([]string{expanded}, saveDir)
				},
			}
			return m, tea.Exec(task, func(err error) tea.Msg {
				if err != nil {
					return opDoneMsg{toast: "上传未全部成功: " + err.Error(), isErr: true, reloadDir: true}
				}
				return opDoneMsg{toast: "上传完成 -> " + saveDir, reloadDir: true}
			})
		}
	case tea.KeySpace:
		if len(msg.Runes) > 0 {
			m.inputValue += sanitizeInputRunes(msg.Runes)
		} else {
			m.inputValue += " "
		}
	case tea.KeyRunes:
		m.inputValue += sanitizeInputRunes(msg.Runes)
	}
	return m, nil
}

func (m *Model) handleBrowseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "q" {
		return m, tea.Quit
	}
	if msg.String() == "?" {
		m.mode = modeHelp
		return m, nil
	}
	if m.loading {
		switch msg.String() {
		case "n":
			m.mode = modeInputMkdir
			m.inputValue = ""
		case "u":
			m.mode = modeInputUpload
			m.inputValue = ""
		case "s":
			m.sortIdx = (m.sortIdx + 1) % len(sortPresets)
			m.statusMsg = "排序方式: " + sortPresetLabel(m.sortIdx, ui.ASCIIOnly())
			m.statusErr = false
			return m, m.loadDirCmd(m.cwd, true)
		case "r":
			m.statusMsg = "正在刷新目录..."
			m.statusErr = false
			return m, tea.Batch(m.loadDirCmd(m.cwd, true), m.loadMetaCmd())
		case "backspace", "left", "h":
			if m.cwd != "/" {
				parent := path.Dir(m.cwd)
				m.cwd = parent
				m.filterQuery = ""
				m.selected = make(map[string]*baidupcs.FileDirectory)
				m.allFiles = nil
				m.visible = nil
				m.cursor = 0
				m.scrollTop = 0
				m.statusMsg = ""
				return m, m.loadDirCmd(parent, false)
			}
		}
		return m, nil
	}

	pageStep := m.listViewportRows() / 2
	if pageStep < 3 {
		pageStep = 3
	}

	switch msg.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+u":
		m.moveCursor(-pageStep)
	case "pgdown", "ctrl+d":
		m.moveCursor(pageStep)
	case "home", "g":
		m.cursor = 0
		m.clampCursor()
	case "end", "G":
		if len(m.visible) > 0 {
			m.cursor = len(m.visible) - 1
		}
		m.clampCursor()

	case "enter", "right", "l":
		cur := m.CurrentItem()
		if cur != nil && cur.Isdir {
			target := path.Clean(cur.Path)
			m.lastDirPos[m.cwd] = m.cursor
			m.cwd = target
			m.filterQuery = ""
			m.selected = make(map[string]*baidupcs.FileDirectory)
			m.allFiles = nil
			m.visible = nil
			m.cursor = 0
			m.scrollTop = 0
			m.loading = true
			m.statusMsg = ""
			return m, m.loadDirCmd(target, false)
		}

	case "backspace", "left", "h":
		if m.cwd != "/" {
			m.lastDirPos[m.cwd] = m.cursor
			parent := path.Dir(m.cwd)
			m.cwd = parent
			m.filterQuery = ""
			m.selected = make(map[string]*baidupcs.FileDirectory)
			m.allFiles = nil
			m.visible = nil
			m.cursor = 0
			m.scrollTop = 0
			m.loading = true
			m.statusMsg = ""
			return m, m.loadDirCmd(parent, false)
		}

	case " ":
		if cur := m.CurrentItem(); cur != nil {
			if _, exists := m.selected[cur.Path]; exists {
				delete(m.selected, cur.Path)
			} else {
				m.selected[cur.Path] = cur
			}
			m.moveCursor(1)
		}

	case "a":
		if len(m.visible) > 0 {
			allSelected := true
			for _, f := range m.visible {
				if _, ok := m.selected[f.Path]; !ok {
					allSelected = false
					break
				}
			}
			if allSelected {
				for _, f := range m.visible {
					delete(m.selected, f.Path)
				}
			} else {
				for _, f := range m.visible {
					m.selected[f.Path] = f
				}
			}
		}

	case "/":
		m.mode = modeFilter

	case "esc":
		if m.filterQuery != "" {
			m.filterQuery = ""
			m.applyFilter()
		} else if len(m.selected) > 0 {
			m.selected = make(map[string]*baidupcs.FileDirectory)
			m.statusMsg = "已清空多选列表"
			m.statusErr = false
		}

	case "s":
		m.sortIdx = (m.sortIdx + 1) % len(sortPresets)
		m.loading = true
		m.statusMsg = "排序方式: " + sortPresetLabel(m.sortIdx, ui.ASCIIOnly())
		m.statusErr = false
		return m, m.loadDirCmd(m.cwd, true)

	case "r":
		m.loading = true
		m.statusMsg = "正在刷新目录..."
		m.statusErr = false
		return m, tea.Batch(m.loadDirCmd(m.cwd, true), m.loadMetaCmd())

	case "n":
		m.mode = modeInputMkdir
		m.inputValue = ""

	case "R":
		if cur := m.CurrentItem(); cur != nil {
			m.mode = modeInputRename
			m.renameSource = cur.Path
			m.inputValue = cur.Filename
		}

	case "u":
		m.mode = modeInputUpload
		m.inputValue = ""

	case "x", "delete":
		if len(m.actionPaths()) > 0 {
			m.mode = modeConfirmDelete
		}

	case "d":
		targets := m.actionPaths()
		if len(targets) == 0 {
			return m, nil
		}
		count := len(targets)
		task := &inProcessTask{
			fn: func() error {
				return m.backend.Download(targets)
			},
		}
		return m, tea.Exec(task, func(err error) tea.Msg {
			if err != nil {
				return opDoneMsg{toast: "下载出现错误: " + err.Error(), isErr: true}
			}
			return opDoneMsg{
				toast:       fmt.Sprintf("已完成 %d 项下载任务 (保存至 %s)", count, m.saveDir),
				clearSelect: true,
			}
		})
	}

	return m, nil
}

// CurrentItem 返回当前光标所在的文件/目录项。
func (m *Model) CurrentItem() *baidupcs.FileDirectory {
	if m.loading || len(m.visible) == 0 || m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return m.visible[m.cursor]
}

func (m *Model) actionPaths() []string {
	if m.loading {
		return nil
	}
	if len(m.selected) > 0 {
		out := make([]string, 0, len(m.selected))
		for p := range m.selected {
			out = append(out, p)
		}
		sort.Strings(out)
		return out
	}
	if cur := m.CurrentItem(); cur != nil {
		return []string{cur.Path}
	}
	return nil
}

func (m *Model) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.filterQuery))
	if q == "" {
		m.visible = m.allFiles
		m.clampCursor()
		return
	}
	filtered := make(baidupcs.FileDirectoryList, 0, len(m.allFiles))
	for _, f := range m.allFiles {
		if f != nil && strings.Contains(strings.ToLower(f.Filename), q) {
			filtered = append(filtered, f)
		}
	}
	m.visible = filtered
	m.cursor = 0
	m.clampCursor()
}

func (m *Model) moveCursor(delta int) {
	if len(m.visible) == 0 {
		m.cursor = 0
		m.scrollTop = 0
		return
	}
	m.cursor += delta
	m.clampCursor()
}

func (m *Model) listViewportRows() int {
	if m.height > 0 && m.height < 12 {
		if r := m.height - 7; r >= 1 {
			return r
		}
		return 1
	}
	rows := m.height - 9
	if rows < 5 {
		rows = 5
	}
	return rows
}

func (m *Model) clampCursor() {
	if len(m.visible) == 0 {
		m.cursor = 0
		m.scrollTop = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	vp := m.listViewportRows()
	if m.cursor < m.scrollTop {
		m.scrollTop = m.cursor
	}
	if m.cursor >= m.scrollTop+vp {
		m.scrollTop = m.cursor - vp + 1
	}
	if m.scrollTop < 0 {
		m.scrollTop = 0
	}
}

func sortDirsFirst(files baidupcs.FileDirectoryList) baidupcs.FileDirectoryList {
	dirs := make(baidupcs.FileDirectoryList, 0, len(files))
	regular := make(baidupcs.FileDirectoryList, 0, len(files))
	for _, f := range files {
		if f == nil {
			continue
		}
		if f.Isdir {
			dirs = append(dirs, f)
		} else {
			regular = append(regular, f)
		}
	}
	return append(dirs, regular...)
}
