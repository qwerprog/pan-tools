package checksum_test

import (
	"bytes"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/checksum"
	"os"
	"path/filepath"
	"testing"
)

var (
	flagList = []int{
		checksum.CHECKSUM_MD5,
		checksum.CHECKSUM_SLICE_MD5,
		checksum.CHECKSUM_CRC32,
		checksum.CHECKSUM_MD5 | checksum.CHECKSUM_SLICE_MD5,
		checksum.CHECKSUM_SLICE_MD5 | checksum.CHECKSUM_CRC32,
		checksum.CHECKSUM_MD5 | checksum.CHECKSUM_CRC32,
		checksum.CHECKSUM_MD5 | checksum.CHECKSUM_SLICE_MD5 | checksum.CHECKSUM_CRC32,
	}
)

func TestChecksum(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "sample.bin")
	data := bytes.Repeat([]byte("pan-tools checksum test data\n"), 1024)
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	for _, flag := range flagList {
		lf, err := checksum.GetFileSum(tmpFile, flag)
		if err != nil {
			t.Fatal(err)
		}
		if flag&checksum.CHECKSUM_MD5 != 0 && len(lf.MD5) == 0 {
			t.Fatal("expected MD5 to be computed")
		}
	}

	for _, flag := range flagList {
		lf := checksum.NewLocalFileChecksumWithBufSize(tmpFile, checksum.DefaultBufSize-3, checksum.DefaultBufSize)
		err := lf.OpenPath()
		if err != nil {
			t.Fatal(err)
		}
		if err := lf.Sum(flag); err != nil {
			lf.Close()
			t.Fatal(err)
		}
		lf.Close()
	}
}

func TestSumPreservesReadErrorAndSkipsPartialResult(t *testing.T) {
	// Opening a directory produces a seekable *os.File whose Read fails (EISDIR) while Seek(0, 0) succeeds.
	dirPath := t.TempDir()
	lfDir := checksum.NewLocalFileChecksum(dirPath, int(checksum.DefaultBufSize))
	if err := lfDir.OpenPath(); err == nil {
		err = lfDir.Sum(checksum.CHECKSUM_MD5 | checksum.CHECKSUM_CRC32)
		lfDir.Close()
		if err == nil {
			t.Fatal("expected Sum on directory fd to preserve Read error instead of being masked by Seek(0,0)")
		}
		if len(lfDir.MD5) != 0 || lfDir.CRC32 != 0 {
			t.Fatalf("expected partial hashes to remain unset on error, got MD5=%x CRC32=%d", lfDir.MD5, lfDir.CRC32)
		}
	}

	tmpFile := filepath.Join(t.TempDir(), "closed.bin")
	if err := os.WriteFile(tmpFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	lf := checksum.NewLocalFileChecksum(tmpFile, int(checksum.DefaultBufSize))
	if err := lf.OpenPath(); err != nil {
		t.Fatal(err)
	}
	// Truncate after OpenPath so Length > actual file size -> CalculateChunkedSum hits early EOF
	if err := os.Truncate(tmpFile, 2); err != nil {
		lf.Close()
		t.Fatal(err)
	}
	err := lf.CalculateChunkedSum(64)
	lf.Close()
	if err == nil {
		t.Fatal("expected error when file is shorter than recorded Length, got nil")
	}
}
