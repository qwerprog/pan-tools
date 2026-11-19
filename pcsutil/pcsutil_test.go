package pcsutil

import (
	"bytes"
	"compress/gzip"
	"path/filepath"
	"testing"
)

func TestDecompressGZIP(t *testing.T) {
	want := []byte("pan-tools compressed payload verification")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(want); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := DecompressGZIP(&buf)
	if err != nil {
		t.Fatalf("DecompressGZIP failed: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("DecompressGZIP = %q, want %q", got, want)
	}
}

func TestExpandLocalPathQuotesAndTilde(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	if got := ExpandLocalPath("  '~/Movies/clip 1.mp4'  "); got != filepath.Join(fakeHome, "Movies/clip 1.mp4") {
		t.Fatalf("ExpandLocalPath single-quoted tilde = %q, want %q", got, filepath.Join(fakeHome, "Movies/clip 1.mp4"))
	}
	if got := ExpandLocalPath(`"~"`); got != fakeHome {
		t.Fatalf("ExpandLocalPath(\"~\") = %q, want %q", got, fakeHome)
	}
	if got := ExpandLocalPath(`"/tmp/my archive.zip"`); got != "/tmp/my archive.zip" {
		t.Fatalf("ExpandLocalPath double-quoted = %q, want /tmp/my archive.zip", got)
	}
	if got := ExpandLocalPath("   "); got != "" {
		t.Fatalf("ExpandLocalPath whitespace = %q, want empty", got)
	}
}
