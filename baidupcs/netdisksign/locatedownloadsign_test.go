package netdisksign_test

import (
	"strings"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/netdisksign"
)

func TestLocateDownloadSign(t *testing.T) {
	sign := netdisksign.NewLocateDownloadSignWithTimeAndDevUID(
		1571140066,
		"O|1E67351CCE80B2CF48DB511CD77ACD9F",
		10086,
		"test_bduss",
	)
	const wantRand = "b6bb7a6f46899e181baea58798d4fdb889775c2c"
	if sign.Rand != wantRand {
		t.Fatalf("sign.Rand = %q, want %q", sign.Rand, wantRand)
	}

	param := sign.URLParam()
	for _, token := range []string{"rand=" + wantRand, "time=1571140066", "devuid="} {
		if !strings.Contains(param, token) {
			t.Fatalf("URLParam() = %q, missing %q", param, token)
		}
	}
}
