package ui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

// TransferSlot 描述多任务实时面板中的单个活跃传输项快照。
type TransferSlot struct {
	ID         string
	Name       string
	Arrow      string
	DoneBytes  int64
	TotalBytes int64
	SpeedBytes int64
	Elapsed    time.Duration
	ETA        string
}

// TransferDashboard 在多文件并发上传/下载（load > 1）时维护底部固定高度的 ANSI 实时传输看板，避免逐秒滚屏。
type TransferDashboard struct {
	mu            sync.Mutex
	out           io.Writer
	title         string
	totalTasks    int
	completed     int
	maxVisible    int
	enabled       bool
	color         bool
	ascii         bool
	startTime     time.Time
	lastRender    time.Time
	renderedLines int
	slots         map[string]TransferSlot
}

// NewTransferDashboard 创建一个并发安全的多任务实时传输看板。
// 当非 TTY、开启 --quiet 或仅单文件任务时自动退化为普通逐行输出，不发送多行光标控制序列。
func NewTransferDashboard(out io.Writer, title string, totalTasks int, multiTask bool) *TransferDashboard {
	if out == nil {
		out = os.Stdout
	}
	tty := IsTTY(out) && !IsQuiet() && multiTask && totalTasks > 1
	return &TransferDashboard{
		out:        out,
		title:      title,
		totalTasks: totalTasks,
		maxVisible: 5,
		enabled:    tty,
		color:      ColorEnabled(out),
		ascii:      ASCIIOnly(),
		startTime:  time.Now(),
		slots:      make(map[string]TransferSlot),
	}
}

// Enabled 返回是否启用原地多行动态重绘面板。
func (d *TransferDashboard) Enabled() bool {
	if d == nil {
		return false
	}
	return d.enabled
}

// UpdateSlot 更新某个活跃传输任务的状态并按节流频率重绘面板。
func (d *TransferDashboard) UpdateSlot(slot TransferSlot) {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	d.slots[slot.ID] = slot
	if time.Since(d.lastRender) < 75*time.Millisecond {
		return
	}
	d.renderLocked()
}

// CompleteSlot 移除已完成/跳过的任务，在动态面板上方永久打印一行完成摘要并重绘剩余活跃任务。
func (d *TransferDashboard) CompleteSlot(id string, permanentLine string) {
	if d == nil {
		if permanentLine != "" && !IsQuiet() {
			fmt.Println(permanentLine)
		}
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.slots, id)
	d.completed++

	if !d.enabled {
		if permanentLine != "" && !IsQuiet() {
			fmt.Fprintln(d.out, permanentLine)
		}
		return
	}

	d.clearLocked()
	if permanentLine != "" {
		fmt.Fprintln(d.out, permanentLine)
	}
	if len(d.slots) > 0 {
		d.renderLocked()
	}
}

// ClearSlot 仅从活跃面板移除某任务（例如准备重试或失败时），不计入完成数。
func (d *TransferDashboard) ClearSlot(id string, logLine string) {
	if d == nil {
		if logLine != "" {
			fmt.Fprintln(os.Stderr, logLine)
		}
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.slots, id)
	if !d.enabled {
		if logLine != "" {
			fmt.Fprintln(os.Stderr, logLine)
		}
		return
	}
	d.clearLocked()
	if logLine != "" {
		fmt.Fprintln(os.Stderr, logLine)
	}
	if len(d.slots) > 0 {
		d.renderLocked()
	}
}

// Close 结束传输任务时清理底部临时动态行。
func (d *TransferDashboard) Close() {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clearLocked()
	d.slots = make(map[string]TransferSlot)
}

func (d *TransferDashboard) clearLocked() {
	if d.renderedLines <= 0 {
		return
	}
	fmt.Fprintf(d.out, "\x1b[%dA\r\x1b[J", d.renderedLines)
	d.renderedLines = 0
}

func (d *TransferDashboard) renderLocked() {
	d.clearLocked()
	lines := d.BuildLines(TerminalWidth())
	if len(lines) == 0 {
		return
	}
	for _, line := range lines {
		fmt.Fprintln(d.out, line)
	}
	d.renderedLines = len(lines)
	d.lastRender = time.Now()
}

// BuildLines 构造当前看板的每一行文本（导出以便单元测试）。
func (d *TransferDashboard) BuildLines(maxWidth int) []string {
	if len(d.slots) == 0 {
		return nil
	}
	if maxWidth <= 0 {
		maxWidth = 80
	}

	r := NewRenderer(d.out, &d.color)
	muted := r.NewStyle().Foreground(Palette.Muted)
	titleStyle := r.NewStyle().Foreground(Palette.Primary).Bold(true)
	speedStyle := r.NewStyle().Foreground(Palette.Success).Bold(true)
	nameStyle := r.NewStyle().Foreground(Palette.VaultCore)

	keys := make([]string, 0, len(d.slots))
	var totalSpeed int64
	for k, s := range d.slots {
		keys = append(keys, k)
		totalSpeed += s.SpeedBytes
	}
	sort.Slice(keys, func(i, j int) bool {
		ai, errI := strconv.Atoi(keys[i])
		aj, errJ := strconv.Atoi(keys[j])
		if errI == nil && errJ == nil && ai != aj {
			return ai < aj
		}
		return keys[i] < keys[j]
	})

	ellipsis := "…"
	titleIcon := "⚡ "
	dotPrefix := "· "
	midSep := " · "
	extraLead := "  … "
	if d.ascii {
		ellipsis = "..."
		titleIcon = "[*] "
		dotPrefix = "| "
		midSep = " | "
		extraLead = "  ... "
	}

	var overallRatio float64
	if d.totalTasks > 0 {
		overallRatio = float64(d.completed) / float64(d.totalTasks)
	}
	headBar := RenderBar(d.out, overallRatio, 12, Palette.Primary, d.color, d.ascii)
	elapsed := time.Since(d.startTime).Truncate(time.Second)

	header := fmt.Sprintf("%s %s/%d  %s  %s %s",
		titleStyle.Render(titleIcon+d.title),
		r.NewStyle().Foreground(Palette.Accent).Bold(true).Render(fmt.Sprintf("%d", d.completed)),
		d.totalTasks,
		headBar,
		speedStyle.Render(converter.ConvertFileSize(totalSpeed, 2)+"/s"),
		muted.Render(dotPrefix+elapsed.String()),
	)

	lines := []string{ansi.Truncate(header, maxWidth-1, ellipsis)}

	shown := keys
	extra := 0
	if len(shown) > d.maxVisible {
		extra = len(shown) - d.maxVisible
		shown = shown[:d.maxVisible]
	}

	nameColW := 22
	if maxWidth < 88 {
		nameColW = 14
	}

	for _, id := range shown {
		s := d.slots[id]
		var ratio float64
		if s.TotalBytes > 0 {
			ratio = float64(s.DoneBytes) / float64(s.TotalBytes)
		}
		tone := Palette.Accent
		if s.Arrow == "↓" || s.Arrow == "v" {
			tone = Palette.Primary
		}
		bar := RenderBar(d.out, ratio, 12, tone, d.color, d.ascii)
		shortName := TruncateMiddleASCII(s.Name, nameColW, d.ascii)
		pad := nameColW - lipgloss.Width(shortName)
		if pad < 0 {
			pad = 0
		}
		paddedName := nameStyle.Render(shortName) + strings.Repeat(" ", pad)

		etaPart := ""
		if s.ETA != "" && s.ETA != "-" {
			etaPart = midSep + s.ETA
		}
		row := fmt.Sprintf("  %s %s %s  %s %5.1f%%  %s/%s%s%s/s%s",
			muted.Render("["+s.ID+"]"),
			r.NewStyle().Foreground(tone).Bold(true).Render(s.Arrow),
			paddedName,
			bar,
			clampRatio(ratio)*100,
			converter.ConvertFileSize(s.DoneBytes, 1),
			converter.ConvertFileSize(s.TotalBytes, 1),
			midSep,
			converter.ConvertFileSize(s.SpeedBytes, 1),
			etaPart,
		)
		lines = append(lines, ansi.Truncate(row, maxWidth-1, ellipsis))
	}

	if extra > 0 {
		extraLine := muted.Render(fmt.Sprintf("%s另有 %d 个任务正在并发传输", extraLead, extra))
		lines = append(lines, ansi.Truncate(extraLine, maxWidth-1, ellipsis))
	}
	return lines
}
