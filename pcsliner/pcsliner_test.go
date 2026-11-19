package pcsliner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPCSLinerPauseResumeNonTTYAndHistoryClose(t *testing.T) {
	pl := &PCSLiner{}
	if err := pl.Pause(); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	// Repeated Pause must be idempotent rather than panicking.
	if err := pl.Pause(); err != nil {
		t.Fatalf("second Pause failed: %v", err)
	}
	if err := pl.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if err := pl.Resume(); err != nil {
		t.Fatalf("second Resume failed: %v", err)
	}

	histPath := filepath.Join(t.TempDir(), "history.txt")
	lh, err := NewLineHistory(histPath)
	if err != nil {
		t.Fatalf("NewLineHistory failed: %v", err)
	}
	pl = NewLiner()
	pl.History = lh
	for i := 0; i < 3; i++ {
		if err := pl.DoWriteHistory(); err != nil {
			t.Fatalf("DoWriteHistory iteration %d failed: %v", i, err)
		}
	}
	if err := pl.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(histPath)
		if err != nil {
			t.Fatalf("Stat history failed: %v", err)
		}
		if perm := fi.Mode().Perm(); perm != 0600 {
			t.Fatalf("history file perm = %04o, want 0600", perm)
		}
	}
}
