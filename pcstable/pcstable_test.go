package pcstable

import (
	"bytes"
	"strings"
	"testing"
)

func TestPCSTableRender(t *testing.T) {
	var buf bytes.Buffer
	tb := NewTable(&buf)
	tb.SetHeader([]string{"#", "名称", "大小"})
	tb.SetColumnAlignment([]int{AlignDefault, AlignLeft, AlignRight})
	tb.Append([]string{"0", "hello.txt", "12 KB"})
	tb.AppendBulk([][]string{{"1", "docs/", "-"}})
	tb.Render()

	out := buf.String()
	for _, want := range []string{"名称", "hello.txt", "12 KB", "docs/"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, out)
		}
	}

	// Headerless PCSTable must also render all rows including the last row.
	buf.Reset()
	kv := NewTable(&buf)
	kv.SetColumnAlignment([]int{AlignLeft, AlignLeft})
	kv.AppendBulk([][]string{
		{"文件大小", "1024"},
		{"md5", "abc"},
		{"前256KB切片的md5", "abc"},
		{"crc32", "12345678"},
	})
	kv.Render()
	if !strings.Contains(buf.String(), "crc32") || !strings.Contains(buf.String(), "12345678") {
		t.Fatalf("expected headerless PCSTable to preserve final crc32 row, got:\n%s", buf.String())
	}
}
