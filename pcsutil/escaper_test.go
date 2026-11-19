package pcsutil_test

import (
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"testing"
)

func TestEscape(t *testing.T) {
	got := pcsutil.Escape(`asdf'asdfasd[]a[\[][sdf\[d]`, []rune{'[', '\''})
	if got == "" {
		t.Fatal("expected non-empty escaped string")
	}
}
