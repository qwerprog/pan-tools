package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

func (m *Model) View() string {
	colorOn := ui.ColorEnabled(os.Stdout)
	asciiOn := ui.ASCIIOnly()
	r := ui.NewRenderer(os.Stdout, &colorOn)

	w := m.width
	if w <= 0 {
		w = 80
	}

	header := m.renderHeader(r, w, colorOn, asciiOn)
	body := m.renderBody(r, w, colorOn, asciiOn)
	footer := m.renderFooter(r, w, asciiOn)

	if m.mode == modeHelp {
		return lipgloss.JoinVertical(lipgloss.Left, header, m.renderHelpCard(r, w, asciiOn), footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *Model) renderHeader(r *lipgloss.Renderer, totalW int, colorOn, asciiOn bool) string {
	brandText := "◈ pan-tools Explorer"
	ellipsis := "…"
	if asciiOn {
		brandText = "* pan-tools Explorer"
		ellipsis = "..."
	}
	brand := r.NewStyle().Foreground(ui.Palette.Primary).Bold(true).Render(brandText)
	muted := r.NewStyle().Foreground(ui.Palette.Muted)
	accent := r.NewStyle().Foreground(ui.Palette.Accent).Bold(true)

	userPart := ""
	if cleanUser := sanitizeSingleLine(m.userName); cleanUser != "" {
		dot := r.NewStyle().Foreground(ui.Palette.Success).Render("●")
		if asciiOn {
			dot = "*"
		}
		userPart = dot + " " + r.NewStyle().Foreground(ui.Palette.VaultCore).Bold(true).Render(cleanUser)
		if cleanVIP := sanitizeSingleLine(m.vipLabel); cleanVIP != "" {
			kind := ui.BadgeOffline
			if m.isSVIP {
				kind = ui.BadgeWarn
			} else if m.isVIP {
				kind = ui.BadgeInfo
			}
			userPart += " " + ui.RenderBadge(os.Stdout, kind, cleanVIP, colorOn)
		}
	}

	quotaPart := ""
	if m.hasQuota && m.quotaTotal > 0 {
		ratio := float64(m.quotaUsed) / float64(m.quotaTotal)
		bar := ui.RenderQuotaBar(os.Stdout, ratio, 10, colorOn, asciiOn)
		quotaPart = fmt.Sprintf("%s %s",
			bar,
			muted.Render(fmt.Sprintf("(%s/%s)",
				converter.ConvertFileSize(m.quotaUsed, 1),
				converter.ConvertFileSize(m.quotaTotal, 1),
			)),
		)
	}

	topLeft := brand
	if userPart != "" {
		topLeft += "  " + userPart
	}
	maxLineW := maxInt(1, totalW-2)
	gap := totalW - lipgloss.Width(topLeft) - lipgloss.Width(quotaPart) - 2
	var line1Content string
	if gap >= 2 && quotaPart != "" {
		line1Content = topLeft + strings.Repeat(" ", gap) + quotaPart
	} else if quotaPart != "" {
		line1Content = topLeft + "  " + quotaPart
	} else {
		line1Content = topLeft
	}
	line1 := " " + ansi.Truncate(line1Content, maxLineW, ellipsis)

	sortTag := muted.Render("排序: ") + r.NewStyle().Foreground(ui.Palette.Secondary).Render(sortPresetLabel(m.sortIdx, asciiOn))
	filterTag := ""
	if m.filterQuery != "" {
		filterTag = "  " + r.NewStyle().Foreground(ui.Palette.Warn).Render("过滤: "+sanitizeSingleLine(m.filterQuery))
	}
	pathBudget := maxInt(8, totalW-34)
	pathLabel := muted.Render("路径 ") + accent.Render(ui.TruncateMiddleASCII(sanitizeSingleLine(m.cwd), pathBudget, asciiOn))
	gap2 := totalW - lipgloss.Width(pathLabel) - lipgloss.Width(sortTag+filterTag) - 2
	var line2Content string
	if gap2 >= 2 {
		line2Content = pathLabel + strings.Repeat(" ", gap2) + sortTag + filterTag
	} else {
		line2Content = pathLabel + "  " + sortTag + filterTag
	}
	line2 := " " + ansi.Truncate(line2Content, maxLineW, ellipsis)

	return line1 + "\n" + line2
}

func (m *Model) renderBody(r *lipgloss.Renderer, totalW int, colorOn, asciiOn bool) string {
	border := lipgloss.RoundedBorder()
	if asciiOn {
		border = lipgloss.ASCIIBorder()
	}

	vpRows := m.listViewportRows()
	showInspector := totalW >= 94

	listInnerW := totalW - 4
	if listInnerW < 1 {
		listInnerW = 1
	}
	inspectorInnerW := 0
	if showInspector {
		inspectorInnerW = totalW*36/100 - 4
		if inspectorInnerW < 30 {
			inspectorInnerW = 30
		}
		listInnerW = totalW - inspectorInnerW - 8
	}

	listPane := m.renderListPane(r, listInnerW, vpRows, colorOn, asciiOn)
	leftBox := r.NewStyle().
		Border(border).
		BorderForeground(ui.Palette.Hairline).
		Padding(0, 1).
		Width(listInnerW + 2).
		Height(vpRows + 1).
		Render(listPane)

	if !showInspector {
		return leftBox
	}

	rightPane := m.renderInspectorPane(r, inspectorInnerW, vpRows, colorOn, asciiOn)
	rightBox := r.NewStyle().
		Border(border).
		BorderForeground(ui.Palette.Hairline).
		Padding(0, 1).
		Width(inspectorInnerW + 2).
		Height(vpRows + 1).
		Render(rightPane)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
}

func (m *Model) renderListPane(r *lipgloss.Renderer, innerW, vpRows int, colorOn, asciiOn bool) string {
	muted := r.NewStyle().Foreground(ui.Palette.Muted)
	headerStyle := r.NewStyle().Foreground(ui.Palette.Primary).Bold(true)
	ellipsis := "…"
	if asciiOn {
		ellipsis = "..."
	}

	sizeW := 10
	dateW := 16
	if innerW < 58 {
		dateW = 0
	}
	if innerW < 34 {
		sizeW = 0
	}
	nameW := innerW - 3
	if sizeW > 0 {
		nameW -= 1 + sizeW
	}
	if dateW > 0 {
		nameW -= 2 + dateW
	}
	if nameW < 1 {
		nameW = 1
	}

	var lines []string
	countBadge := fmt.Sprintf("[%d/%d]", minInt(m.cursor+1, len(m.visible)), len(m.visible))
	hdrName := padRight(ansi.Truncate("名称 "+countBadge, nameW+3, ellipsis), nameW+3)
	hdrLine := headerStyle.Render(hdrName)
	if sizeW > 0 {
		hdrLine += " " + headerStyle.Render(padLeft("大小", sizeW))
	}
	if dateW > 0 {
		hdrLine += "  " + headerStyle.Render(padRight("修改时间", dateW))
	}
	lines = append(lines, ansi.Truncate(hdrLine, innerW, ""))

	if m.loading {
		lines = append(lines, ansi.Truncate(muted.Render("  正在加载目录列表..."), innerW, ellipsis))
		return strings.Join(lines, "\n")
	}

	if len(m.visible) == 0 {
		emptyHint := "  (当前目录为空)"
		if m.filterQuery != "" {
			emptyHint = fmt.Sprintf("  (无匹配 %q 的条目，按 Esc 清除过滤)", sanitizeSingleLine(m.filterQuery))
		}
		lines = append(lines, ansi.Truncate(muted.Render(emptyHint), innerW, ellipsis))
		return strings.Join(lines, "\n")
	}

	end := m.scrollTop + vpRows
	if end > len(m.visible) {
		end = len(m.visible)
	}

	selRowBg := lipgloss.Color("#2C313C")
	for idx := m.scrollTop; idx < end; idx++ {
		f := m.visible[idx]
		isCur := idx == m.cursor
		_, isSel := m.selected[f.Path]

		cursorMark := "  "
		if isCur {
			if asciiOn {
				cursorMark = "> "
			} else {
				cursorMark = r.NewStyle().Foreground(ui.Palette.Accent).Bold(true).Render("❯ ")
			}
		}

		selMark := " "
		if isSel {
			if asciiOn {
				selMark = "*"
			} else {
				selMark = r.NewStyle().Foreground(ui.Palette.Warn).Bold(true).Render("●")
			}
		}

		cleanName := sanitizeSingleLine(f.Filename)
		displayName := cleanName
		if f.Isdir {
			if asciiOn {
				displayName = cleanName + "/"
			} else {
				displayName = "▸ " + cleanName + "/"
			}
		}
		truncName := ui.TruncateMiddleASCII(displayName, nameW, asciiOn)
		namePad := nameW - lipgloss.Width(truncName)
		if namePad < 0 {
			namePad = 0
		}

		catColor := ui.FileCategoryColor(ui.ClassifyFile(cleanName, f.Isdir))
		nameSt := r.NewStyle().Foreground(catColor)
		if f.Isdir || isCur {
			nameSt = nameSt.Bold(true)
		}
		styledName := nameSt.Render(truncName) + strings.Repeat(" ", namePad)

		rowStr := cursorMark + selMark + styledName
		if sizeW > 0 {
			rawSize := "-"
			if !f.Isdir {
				rawSize = converter.ConvertFileSize(f.Size, 1)
			}
			paddedSize := padLeft(ansi.Truncate(rawSize, sizeW, ""), sizeW)
			var sizeRendered string
			if isCur && colorOn {
				sizeRendered = r.NewStyle().Foreground(ui.Palette.VaultCore).Render(paddedSize)
			} else {
				sizeRendered = ui.StyleFileSize(os.Stdout, f.Size, paddedSize, f.Isdir, colorOn)
			}
			rowStr += " " + sizeRendered
		}
		if dateW > 0 {
			shortDate := pcsutil.FormatTime(f.Mtime)
			if idx := strings.LastIndexByte(shortDate, ':'); idx > 0 && strings.Count(shortDate, ":") >= 2 {
				shortDate = shortDate[:idx]
			}
			paddedDate := padRight(ansi.Truncate(shortDate, dateW, ""), dateW)
			dateSt := muted
			if isCur {
				dateSt = r.NewStyle().Foreground(ui.Palette.Subtitle)
			}
			rowStr += "  " + dateSt.Render(paddedDate)
		}

		rowStr = ansi.Truncate(rowStr, innerW, "")
		if isCur && colorOn {
			padW := innerW - lipgloss.Width(rowStr)
			if padW > 0 {
				rowStr += strings.Repeat(" ", padW)
			}
			rowStr = r.NewStyle().Background(selRowBg).Render(rowStr)
		}
		lines = append(lines, rowStr)
	}

	return strings.Join(lines, "\n")
}

func (m *Model) renderInspectorPane(r *lipgloss.Renderer, innerW, vpRows int, colorOn, asciiOn bool) string {
	title := r.NewStyle().Foreground(ui.Palette.Primary).Bold(true).Render("详情预览")
	muted := r.NewStyle().Foreground(ui.Palette.Muted)
	valStyle := r.NewStyle().Foreground(ui.Palette.VaultCore)
	accent := r.NewStyle().Foreground(ui.Palette.Accent)
	ruleChar := "─"
	ellipsis := "…"
	if asciiOn {
		ruleChar = "-"
		ellipsis = "..."
	}
	divider := r.NewStyle().Foreground(ui.Palette.Hairline).Render(strings.Repeat(ruleChar, innerW))

	lines := []string{title, divider}

	cur := m.CurrentItem()
	if cur == nil {
		lines = append(lines, muted.Render("未选中任何文件或目录"))
	} else {
		kindStr := "文件"
		nameExtra := 0
		if cur.Isdir {
			kindStr = "文件夹"
			nameExtra = 3
			if asciiOn {
				nameExtra = 1
			}
		}
		cleanCurName := sanitizeSingleLine(cur.Filename)
		cleanCurPath := sanitizeSingleLine(cur.Path)
		lines = append(lines,
			muted.Render("名称: ")+ui.StyleFileName(os.Stdout, ui.TruncateMiddleASCII(cleanCurName, maxInt(1, innerW-6-nameExtra), asciiOn), cur.Isdir, colorOn, asciiOn),
			muted.Render("类型: ")+valStyle.Render(kindStr),
			muted.Render("路径: ")+accent.Render(ui.TruncateMiddleASCII(cleanCurPath, maxInt(1, innerW-6), asciiOn)),
		)
		if !cur.Isdir {
			lines = append(lines,
				muted.Render("大小: ")+valStyle.Render(fmt.Sprintf("%s (%s B)", converter.ConvertFileSize(cur.Size, 2), strconv.FormatInt(cur.Size, 10))),
			)
		}
		lines = append(lines,
			muted.Render("修改: ")+valStyle.Render(pcsutil.FormatTime(cur.Mtime)),
			muted.Render("创建: ")+muted.Render(pcsutil.FormatTime(cur.Ctime)),
			muted.Render("FSID: ")+muted.Render(strconv.FormatInt(cur.FsID, 10)),
		)
		if !cur.Isdir && cur.MD5 != "" {
			md5Note := sanitizeSingleLine(cur.MD5)
			if len(cur.BlockList) > 1 {
				md5Note = "(多片) " + md5Note
			}
			lines = append(lines, muted.Render("MD5 : ")+muted.Render(ui.TruncateMiddleASCII(md5Note, maxInt(1, innerW-6), asciiOn)))
		}
	}

	lines = append(lines, "", divider)
	var selBytes int64
	for _, f := range m.selected {
		if f != nil && !f.Isdir {
			selBytes += f.Size
		}
	}
	if len(m.selected) > 0 {
		lines = append(lines,
			r.NewStyle().Foreground(ui.Palette.Warn).Bold(true).Render(
				fmt.Sprintf("已多选 %d 项 (%s)", len(m.selected), converter.ConvertFileSize(selBytes, 2)),
			),
		)
	} else {
		lines = append(lines, muted.Render("多选篮: 0 项 (按 Space 多选)"))
	}
	lines = append(lines, muted.Render("下载至: ")+accent.Render(ui.TruncateMiddleASCII(sanitizeSingleLine(m.saveDir), maxInt(1, innerW-8), asciiOn)))

	if len(lines) > vpRows+1 {
		lines = lines[:vpRows+1]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], innerW, ellipsis)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderFooter(r *lipgloss.Renderer, totalW int, asciiOn bool) string {
	muted := r.NewStyle().Foreground(ui.Palette.Muted)
	keySt := r.NewStyle().Foreground(ui.Palette.Primary).Bold(true)
	warnSt := r.NewStyle().Foreground(ui.Palette.Warn).Bold(true)
	errSt := r.NewStyle().Foreground(ui.Palette.Fail).Bold(true)
	okSt := r.NewStyle().Foreground(ui.Palette.Success).Bold(true)

	arrow := " ❯ "
	midDot := " · "
	errIcon := "✖ "
	okIcon := "✓ "
	sumSym := "Σ"
	ellipsis := "…"
	navUD := "↑↓/jk"
	navLR := "←→/hl"
	if asciiOn {
		arrow = " > "
		midDot = " | "
		errIcon = "x "
		okIcon = "[OK] "
		sumSym = "SUM"
		ellipsis = "..."
		navUD = "jk"
		navLR = "hl"
	}

	var promptLine string
	switch m.mode {
	case modeFilter:
		promptLine = warnSt.Render(" 过滤当前目录"+arrow) + m.inputValueOrFilter(m.filterQuery, asciiOn) + muted.Render("  (Enter 确认"+midDot+"Esc 清除)")
	case modeInputMkdir:
		promptLine = keySt.Render(" 新建文件夹名"+arrow) + m.inputValueOrFilter(m.inputValue, asciiOn) + muted.Render("  (Enter 创建"+midDot+"Esc 取消)")
	case modeInputRename:
		promptLine = keySt.Render(" 重命名为"+arrow) + m.inputValueOrFilter(m.inputValue, asciiOn) + muted.Render("  (Enter 确认"+midDot+"Esc 取消)")
	case modeInputUpload:
		promptLine = keySt.Render(" 本地上传路径"+arrow) + m.inputValueOrFilter(m.inputValue, asciiOn) + muted.Render("  (Enter 上传至当前目录"+midDot+"Esc 取消)")
	case modeConfirmDelete:
		n := len(m.actionPaths())
		promptLine = errSt.Render(fmt.Sprintf(" ! 确认将选中的 %d 项移入网盘回收站? [y 确认 / N 取消]", n))
	default:
		if cleanStatus := sanitizeSingleLine(m.statusMsg); cleanStatus != "" {
			if m.statusErr {
				promptLine = " " + errSt.Render(errIcon+cleanStatus)
			} else {
				promptLine = " " + okSt.Render(okIcon+cleanStatus)
			}
		} else {
			fN, dN := m.visible.Count()
			promptLine = muted.Render(fmt.Sprintf(" %s 当前显示 %d 文件%s%d 目录 (总大小 %s)%s退出时自动同步工作目录",
				sumSym, fN, midDot, dN, converter.ConvertFileSize(m.visible.TotalSize(), 2), midDot))
		}
	}

	keys := []struct{ k, desc string }{
		{navUD, "移动"},
		{navLR, "进出"},
		{"Space", "多选"},
		{"/", "过滤"},
		{"d", "下载"},
		{"u", "上传"},
		{"n", "新建"},
		{"R", "重命名"},
		{"x", "删除"},
		{"s", "排序"},
		{"r", "刷新"},
		{"?", "帮助"},
		{"q", "退出"},
	}
	var kParts []string
	for _, item := range keys {
		kParts = append(kParts, keySt.Render(item.k)+" "+muted.Render(item.desc))
	}
	sep := muted.Render(midDot)
	keyBar := " " + ansi.Truncate(strings.Join(kParts, sep), maxInt(1, totalW-2), ellipsis)
	return ansi.Truncate(promptLine, maxInt(1, totalW-1), ellipsis) + "\n" + keyBar
}

func (m *Model) renderHelpCard(r *lipgloss.Renderer, totalW int, asciiOn bool) string {
	border := lipgloss.RoundedBorder()
	titleIcon := "◈"
	sep := " · "
	ellipsis := "…"
	if asciiOn {
		border = lipgloss.ASCIIBorder()
		titleIcon = "*"
		sep = " - "
		ellipsis = "..."
	}
	title := r.NewStyle().Foreground(ui.Palette.Primary).Bold(true).Render(titleIcon + " pan-tools 交互式网盘浏览器" + sep + "快捷键指南")
	keySt := r.NewStyle().Foreground(ui.Palette.Accent).Bold(true)
	descSt := r.NewStyle().Foreground(ui.Palette.VaultCore)
	muted := r.NewStyle().Foreground(ui.Palette.Muted)

	rows := [][2]string{
		{"↑ / k , ↓ / j", "上下移动光标"},
		{"PgUp / Ctrl+U , PgDn / Ctrl+D", "半页快速翻滚"},
		{"g / Home , G / End", "跳至首项 / 末项"},
		{"Enter / → / l", "进入选中文件夹"},
		{"Backspace / ← / h", "返回上级目录 (自动记忆光标位置)"},
		{"Space / a / Esc", "切换多选 / 全选当前列表 / 清空多选或过滤"},
		{"/", "实时模糊过滤当前目录文件名"},
		{"s", "切换排序方式 (名称 ↑ / 时间 ↓ / 大小 ↓)"},
		{"r", "强制刷新当前目录与网盘空间配额"},
		{"d / u", "下载选中项到本地 / 上传本地路径至当前目录"},
		{"n / R / x", "新建文件夹 / 重命名当前项 / 移入回收站"},
		{"q / Ctrl+C", "退出浏览器并同步当前路径为工作目录"},
	}
	if asciiOn {
		asciiReplacer := strings.NewReplacer("↑", "^", "↓", "v", "→", "->", "←", "<-")
		for i := range rows {
			rows[i][0] = asciiReplacer.Replace(rows[i][0])
			rows[i][1] = asciiReplacer.Replace(rows[i][1])
		}
	}

	boxContentW := 82
	if totalW-4 < boxContentW {
		boxContentW = totalW - 4
	}
	if boxContentW < 18 {
		boxContentW = maxInt(2, totalW-2)
	}
	hPad := 2
	if boxContentW < 38 {
		hPad = 1
	}
	if boxContentW < 8 {
		hPad = 0
	}
	innerTextW := maxInt(1, boxContentW-hPad*2)

	keyColW := 30
	if innerTextW < 68 {
		keyColW = 22
	}
	if innerTextW < 42 {
		keyColW = maxInt(8, innerTextW/2)
	}

	bodyHeight := m.listViewportRows() + 3
	maxInnerLines := bodyHeight - 2
	if maxInnerLines < 2 {
		maxInnerLines = 2
	}
	// 非紧凑卡片总高 = len(rows) + 4(标题/空行/提示) + 2(上下Padding) + 2(上下Border) = len(rows) + 8
	compact := bodyHeight < len(rows)+8
	ultraCompact := maxInnerLines < len(rows)+2

	var lines []string
	lines = append(lines, ansi.Truncate(title, innerTextW, ellipsis))
	if !compact {
		lines = append(lines, "")
	}
	if ultraCompact {
		halfW := maxInt(10, (innerTextW-2)/2)
		for i := 0; i < len(rows); i += 2 {
			leftPair := keySt.Render(ui.TruncateMiddleASCII(rows[i][0], halfW/2, asciiOn)) + " " + descSt.Render(rows[i][1])
			leftCol := padRight(ansi.Truncate(leftPair, halfW, ellipsis), halfW)
			if i+1 < len(rows) {
				rightPair := keySt.Render(ui.TruncateMiddleASCII(rows[i+1][0], halfW/2, asciiOn)) + " " + descSt.Render(rows[i+1][1])
				rightCol := ansi.Truncate(rightPair, halfW, ellipsis)
				lines = append(lines, ansi.Truncate(leftCol+"  "+rightCol, innerTextW, ellipsis))
			} else {
				lines = append(lines, ansi.Truncate(leftCol, innerTextW, ellipsis))
			}
		}
	} else {
		for _, row := range rows {
			keyCell := keySt.Render(padRight(ui.TruncateMiddleASCII(row[0], keyColW, asciiOn), keyColW))
			line := fmt.Sprintf("%s  %s", keyCell, descSt.Render(row[1]))
			lines = append(lines, ansi.Truncate(line, innerTextW, ellipsis))
		}
	}
	if !compact {
		lines = append(lines, "")
	}
	closeHint := ansi.Truncate(muted.Render("按 ? / Esc / Enter 关闭帮助并返回文件列表"), innerTextW, ellipsis)
	lines = append(lines, closeHint)

	if len(lines) > maxInnerLines {
		if maxInnerLines == 2 {
			lines = []string{lines[0], closeHint}
		} else {
			trimmed := make([]string, 0, maxInnerLines)
			trimmed = append(trimmed, lines[0])
			trimmed = append(trimmed, lines[1:maxInnerLines-1]...)
			trimmed = append(trimmed, closeHint)
			lines = trimmed
		}
	}

	vPad := 1
	if compact {
		vPad = 0
	}

	card := r.NewStyle().
		Border(border).
		BorderForeground(ui.Palette.Primary).
		Padding(vPad, hPad).
		Width(boxContentW).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(totalW, bodyHeight, lipgloss.Center, lipgloss.Center, card)
}

func (m *Model) inputValueOrFilter(s string, asciiOn bool) string {
	clean := sanitizeSingleLine(s)
	if asciiOn {
		return clean + "_"
	}
	return clean + "█"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func padLeft(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sanitizeSingleLine(s string) string {
	if s == "" {
		return ""
	}
	validUTF8 := utf8.ValidString(s)
	if !validUTF8 {
		s = strings.ToValidUTF8(s, "")
	}
	needsClean := false
	for _, ch := range s {
		if ch < 0x20 || (ch >= 0x7f && ch <= 0x9f) || ch == '\u2028' || ch == '\u2029' || isBidiControl(ch) {
			needsClean = true
			break
		}
	}
	if !needsClean {
		return s
	}
	stripped := strings.ReplaceAll(ansi.Strip(s), "\r\n", "\n")
	var sb strings.Builder
	sb.Grow(len(stripped))
	for _, ch := range stripped {
		switch {
		case ch == '\t' || ch == '\n' || ch == '\r' || ch == '\u2028' || ch == '\u2029':
			sb.WriteByte(' ')
		case ch < 0x20 || (ch >= 0x7f && ch <= 0x9f) || isBidiControl(ch):
			continue
		default:
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

func isBidiControl(r rune) bool {
	return r == '\u200e' || r == '\u200f' ||
		(r >= '\u202a' && r <= '\u202e') ||
		(r >= '\u2066' && r <= '\u2069') ||
		r == '\ufeff'
}
