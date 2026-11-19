// Package pcstable 格式化表格包，基于 internal/ui Lipgloss Table 提供统一样式与终端自适应宽度渲染。
package pcstable

import (
	"io"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
)

const (
	// AlignDefault 默认对齐（左对齐）。
	AlignDefault = int(ui.AlignDefault)
	// AlignCenter 居中对齐。
	AlignCenter = int(ui.AlignCenter)
	// AlignRight 右对齐。
	AlignRight = int(ui.AlignRight)
	// AlignLeft 左对齐。
	AlignLeft = int(ui.AlignLeft)
)

// PCSTable 封装基于 Lipgloss 的现代终端表格渲染器。
type PCSTable struct {
	wt          io.Writer
	headers     []string
	rows        [][]string
	colAligns   []ui.TextAlign
	headerAlign *ui.TextAlign
}

// NewTable 创建预设圆角边框与自动列宽限制的表格实例。
func NewTable(wt io.Writer) *PCSTable {
	return &PCSTable{
		wt: wt,
	}
}

// SetHeader 设置表头列标题。
func (t *PCSTable) SetHeader(keys []string) {
	t.headers = append([]string(nil), keys...)
}

// Append 追加单行数据。
func (t *PCSTable) Append(row []string) {
	cp := make([]string, len(row))
	copy(cp, row)
	t.rows = append(t.rows, cp)
}

// AppendBulk 批量追加多行数据。
func (t *PCSTable) AppendBulk(rows [][]string) {
	for _, r := range rows {
		t.Append(r)
	}
}

// SetHeaderAlignment 设置表头水平对齐方式。
func (t *PCSTable) SetHeaderAlignment(align int) {
	ha := ui.TextAlign(align)
	t.headerAlign = &ha
}

// SetColumnAlignment 设置每一列的水平对齐方式。
func (t *PCSTable) SetColumnAlignment(aligns []int) {
	t.colAligns = make([]ui.TextAlign, len(aligns))
	for i, a := range aligns {
		t.colAligns[i] = ui.TextAlign(a)
	}
}

// Render 将表格写入目标 io.Writer。
func (t *PCSTable) Render() {
	if t.wt == nil {
		return
	}
	out := ui.RenderTable(t.wt, ui.TableConfig{
		Headers:     t.headers,
		Rows:        t.rows,
		Alignments:  t.colAligns,
		HeaderAlign: t.headerAlign,
	})
	if out != "" {
		_, _ = io.WriteString(t.wt, out)
	}
}
