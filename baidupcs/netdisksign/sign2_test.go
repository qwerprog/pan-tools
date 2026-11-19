package netdisksign_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/netdisksign"
)

func TestSign2(t *testing.T) {
	const wantB64 = "8RxCbsVeSzn2UjxJAAiV9QQs/WetOj2FJUGwjsMG6SgxFMWlLS/U1Q=="
	wantBytes, err := base64.StdEncoding.DecodeString(wantB64)
	if err != nil {
		t.Fatalf("DecodeString failed: %v", err)
	}

	got := netdisksign.Sign2(
		[]rune("e8c7d729eea7b54551aa594f942decbe"),
		[]rune("37dbe07ade9359c1aa70807e847f768c13360ad2"),
	)
	if !bytes.Equal(got, wantBytes) {
		t.Fatalf("Sign2 mismatch: got base64 %q, want %q", base64.StdEncoding.EncodeToString(got), wantB64)
	}

	emptyKey := netdisksign.Sign2(nil, []rune("abc"))
	if len(emptyKey) != 3 {
		t.Fatalf("Sign2 with empty key returned len %d, want 3", len(emptyKey))
	}
}
