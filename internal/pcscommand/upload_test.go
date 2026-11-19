package pcscommand

import (
	"strings"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
)

func TestUploadFailureError(t *testing.T) {
	if err := uploadFailureError(0); err != nil {
		t.Fatalf("zero failures returned error: %v", err)
	}

	err := uploadFailureError(2)
	if err == nil {
		t.Fatal("expected non-zero failures to return an error")
	}
	if !strings.Contains(err.Error(), "2") {
		t.Fatalf("expected failure count in error, got %v", err)
	}
}

func TestNormalizeUploadOptionsPreservesNoFilenameCheck(t *testing.T) {
	origIgnore := pcsconfig.Config.IgnoreIllegal
	defer func() {
		pcsconfig.Config.IgnoreIllegal = origIgnore
	}()

	pcsconfig.Config.IgnoreIllegal = false
	opt := normalizeUploadOptions(&UploadOptions{NoFilenameCheck: true})
	if !opt.NoFilenameCheck {
		t.Fatal("caller-supplied NoFilenameCheck=true was overwritten when Config.IgnoreIllegal=false")
	}

	pcsconfig.Config.IgnoreIllegal = true
	optFromCfg := normalizeUploadOptions(&UploadOptions{NoFilenameCheck: false})
	if !optFromCfg.NoFilenameCheck {
		t.Fatal("expected NoFilenameCheck to inherit Config.IgnoreIllegal=true when unset")
	}
}
