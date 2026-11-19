package pcsupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSafeExtractPathBlocksZipSlip(t *testing.T) {
	baseDir := t.TempDir()

	got, err := resolveSafeExtractPath(baseDir, "pant-v4.0.2-linux-amd64/pant")
	if err != nil {
		t.Fatalf("unexpected error on valid zip entry: %v", err)
	}
	if got != filepath.Join(baseDir, "pant") {
		t.Fatalf("got %q, want %q", got, filepath.Join(baseDir, "pant"))
	}

	for _, bad := range []string{
		"archive/../../etc/passwd",
		"../../evil.sh",
		"archive//etc/shadow",
		"archive/",
	} {
		if p, err := resolveSafeExtractPath(baseDir, bad); err == nil {
			t.Fatalf("expected error for unsafe zip entry %q, got path %q", bad, p)
		}
	}
}

func TestIsNewerSemVer(t *testing.T) {
	cases := []struct {
		current   string
		candidate string
		want      bool
	}{
		{"v4.0.2", "v4.0.10", true},
		{"v4.0.10", "v4.0.2", false},
		{"v4.0.2-dev", "v4.0.2", true},
		{"v4.0.2", "v4.0.2", false},
		{"v4.0.2+build1", "v4.0.2", false},
		{"v3.9.7", "v4.0.0", true},
		{"v4.0.2", "v4.1.0-Beta1", false},
		{"v4.0.2", "4.1.0", false},
		{"devel", "v4.0.2", true},
	}

	for _, tc := range cases {
		if got := isNewerSemVer(tc.current, tc.candidate); got != tc.want {
			t.Fatalf("isNewerSemVer(%q, %q) = %v, want %v", tc.current, tc.candidate, got, tc.want)
		}
	}
}

func TestResolveUpdateRepoOverride(t *testing.T) {
	t.Setenv("PANT_UPDATE_REPO", "")
	if got := resolveUpdateRepo(); got != DefaultUpdateRepo {
		t.Fatalf("got %q, want %q", got, DefaultUpdateRepo)
	}

	t.Setenv("PANT_UPDATE_REPO", "  custom/pant  ")
	if got := resolveUpdateRepo(); got != "custom/pant" {
		t.Fatalf("got %q, want %q", got, "custom/pant")
	}
}

func TestBuildReleaseAssetRegexpMatchesTarGzAndZip(t *testing.T) {
	linuxExp := buildReleaseAssetRegexp("v5.0.0", "linux", "amd64")
	if !linuxExp.MatchString("pant-v5.0.0-linux-amd64.tar.gz") {
		t.Fatalf("expected linux regexp to match .tar.gz")
	}
	if linuxExp.MatchString("pant-v5.0.0-linux-arm64.tar.gz") {
		t.Fatalf("expected linux amd64 regexp not to match arm64")
	}

	darwinExp := buildReleaseAssetRegexp("v5.0.0", "darwin", "arm64")
	if !darwinExp.MatchString("pant-v5.0.0-darwin-arm64.zip") {
		t.Fatalf("expected darwin regexp to match .zip")
	}
}

func TestExtractTarGzArchiveAndBlocksTraversal(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	entries := []struct {
		name string
		body string
	}{
		{"pant-v5.0.0-linux-amd64/README.md", "updated readme"},
		{"pant-v5.0.0-linux-amd64/../../evil.txt", "blocked"},
	}
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0644,
			Size:     int64(len(e.body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(destDir, "README.md"), []byte("old readme"), 0644); err != nil {
		t.Fatal(err)
	}
	fileNum, errTimes, err := extractUpdateArchive(destDir, "pant-v5.0.0-linux-amd64.tar.gz", buf.Bytes())
	if err != nil {
		t.Fatalf("extractUpdateArchive failed: %v", err)
	}
	if fileNum != 2 || errTimes != 1 {
		t.Fatalf("got fileNum=%d errTimes=%d, want 2 and 1", fileNum, errTimes)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "README.md"))
	if err != nil || string(got) != "updated readme" {
		t.Fatalf("unexpected extracted README.md: err=%v got=%q", err, string(got))
	}
}
