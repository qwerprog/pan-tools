package pcsutil_test

import (
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"os"
	"path/filepath"
	"testing"
)

func TestWalkDir(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := pcsutil.WalkDir(tmpDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}
