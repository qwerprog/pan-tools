package converter_test

import (
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/converter"
)

func TestParseFileSizeStrAndConvertFileSize(t *testing.T) {
	validCases := []struct {
		in   string
		want int64
	}{
		{"32", 32},
		{"512B", 512},
		{"1k", converter.KB},
		{"2KB", 2 * converter.KB},
		{"3.5mb", int64(3.5 * float64(converter.MB))},
		{"4gb", 4 * converter.GB},
	}

	for _, tc := range validCases {
		got, err := converter.ParseFileSizeStr(tc.in)
		if err != nil {
			t.Fatalf("ParseFileSizeStr(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseFileSizeStr(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}

	for _, invalid := range []string{"", "abc", "12XYZ", ".", "1.2.3MB"} {
		if _, err := converter.ParseFileSizeStr(invalid); err == nil {
			t.Fatalf("expected error for invalid size string %q", invalid)
		}
	}

	if got := converter.ConvertFileSize(-10, 2); got != "0B" {
		t.Fatalf("ConvertFileSize(-10, 2) = %q, want \"0B\"", got)
	}
	if got := converter.ConvertFileSize(1536, 2); got != "1.50KB" {
		t.Fatalf("ConvertFileSize(1536, 2) = %q, want \"1.50KB\"", got)
	}
	if got := converter.ConvertFileSize(2*converter.GB, 1); got != "2.0GB" {
		t.Fatalf("ConvertFileSize(2GB, 1) = %q, want \"2.0GB\"", got)
	}
}
