package downloader_test

import (
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
)

func TestRangeListGenContiguousCoverage(t *testing.T) {
	const total = int64(1024)
	cases := []struct {
		name string
		gen  *transfer.RangeListGen
	}{
		{"default_parallel_10", transfer.NewRangeListGenDefault(total, 0, 0, 10)},
		{"block_size_53", transfer.NewRangeListGenBlockSize(total, 0, 53)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var expectedBegin int64
			count := 0
			for _, r := tc.gen.GenRange(); r != nil; _, r = tc.gen.GenRange() {
				if r.LoadBegin() != expectedBegin {
					t.Fatalf("range %d begin = %d, want %d", count, r.LoadBegin(), expectedBegin)
				}
				if r.LoadEnd() <= r.LoadBegin() {
					t.Fatalf("invalid non-positive range %d: [%d, %d)", count, r.LoadBegin(), r.LoadEnd())
				}
				expectedBegin = r.LoadEnd()
				count++
			}
			if expectedBegin != total {
				t.Fatalf("final range end = %d, want %d", expectedBegin, total)
			}
			if !tc.gen.IsDone() {
				t.Fatal("expected gen.IsDone() == true after exhausting ranges")
			}
		})
	}
}
