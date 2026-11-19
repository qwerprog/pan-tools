package pcsupdate

import (
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
