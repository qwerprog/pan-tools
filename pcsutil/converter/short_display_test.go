package converter_test

import (
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

func TestShortDisplay(t *testing.T) {
	src := "\u0000我我\u0000我我我我"
	cases := []struct {
		maxCols int
		want    string
	}{
		{0, "..."},
		{3, "我..."},
		{4, "我我..."},
		{11, "我我我我我..."},
		{12, "我我我我我我"},
		{20, "我我我我我我"},
	}

	for _, tc := range cases {
		if got := converter.ShortDisplay(src, tc.maxCols); got != tc.want {
			t.Fatalf("ShortDisplay(%q, %d) = %q, want %q", src, tc.maxCols, got, tc.want)
		}
	}
}
