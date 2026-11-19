package pcscommand

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
)

// buildTestQRMatrix 构造满足 ISO/IEC 18004 定位符、分隔带与时序图案的标准 N x N (N=21+4k) 二维码矩阵。
func buildTestQRMatrix(n int, seed uint32) [][]bool {
	m := make([][]bool, n)
	reserved := make([][]bool, n)
	for r := 0; r < n; r++ {
		m[r] = make([]bool, n)
		reserved[r] = make([]bool, n)
	}

	placeFinder := func(ox, oy int) {
		for dy := 0; dy < 7; dy++ {
			for dx := 0; dx < 7; dx++ {
				reserved[oy+dy][ox+dx] = true
				m[oy+dy][ox+dx] = dx == 0 || dx == 6 || dy == 0 || dy == 6 || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4)
			}
		}
	}
	placeFinder(0, 0)
	placeFinder(n-7, 0)
	placeFinder(0, n-7)

	markSep := func(x, y int) {
		if x >= 0 && x < n && y >= 0 && y < n {
			reserved[y][x] = true
			m[y][x] = false
		}
	}
	for i := 0; i < 8; i++ {
		markSep(7, i)
		markSep(i, 7)
		markSep(n-8, i)
		markSep(n-8+i, 7)
		markSep(7, n-8+i)
		markSep(i, n-8)
	}

	for idx := 8; idx <= n-9; idx++ {
		reserved[6][idx] = true
		m[6][idx] = idx%2 == 0
		reserved[idx][6] = true
		m[idx][6] = idx%2 == 0
	}

	x := seed
	if x == 0 {
		x = 0x9e3779b9
	}
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			if reserved[r][c] {
				continue
			}
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
			m[r][c] = (x & 1) == 1
		}
	}
	return m
}

func renderMatrixToPNG(t *testing.T, matrix [][]bool, stride, padLeft, padTop, padRight, padBottom int, transparentBg bool) []byte {
	t.Helper()
	n := len(matrix)
	w := padLeft + n*stride + padRight
	h := padTop + n*stride + padBottom
	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	bg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if transparentBg {
		bg = color.NRGBA{R: 0, G: 0, B: 0, A: 0}
	}
	fg := color.NRGBA{R: 0, G: 0, B: 0, A: 255}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, bg)
		}
	}
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			if !matrix[r][c] {
				continue
			}
			for dy := 0; dy < stride; dy++ {
				for dx := 0; dx < stride; dx++ {
					img.SetNRGBA(padLeft+c*stride+dx, padTop+r*stride+dy, fg)
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode failed: %v", err)
	}
	return buf.Bytes()
}

func unpackDense1x2ToMatrix(rendered string, n int) ([][]bool, error) {
	plain := ansi.Strip(rendered)
	lines := strings.Split(plain, "\n")
	const qz = 2
	totalRows := n + 2*qz
	totalCols := n + 2*qz

	full := make([][]bool, 0, len(lines)*2)
	for li, line := range lines {
		runes := []rune(line)
		if len(runes) != totalCols {
			return nil, fmt.Errorf("line %d rune len=%d, want %d", li, len(runes), totalCols)
		}
		topRow := make([]bool, totalCols)
		botRow := make([]bool, totalCols)
		for ci, ch := range runes {
			switch ch {
			case '█':
				topRow[ci], botRow[ci] = true, true
			case '▀':
				topRow[ci], botRow[ci] = true, false
			case '▄':
				topRow[ci], botRow[ci] = false, true
			case ' ':
				topRow[ci], botRow[ci] = false, false
			default:
				return nil, fmt.Errorf("unexpected glyph %q at (%d,%d)", ch, li, ci)
			}
		}
		full = append(full, topRow, botRow)
	}
	if len(full) < totalRows {
		return nil, fmt.Errorf("unpacked rows=%d < totalRows=%d", len(full), totalRows)
	}

	// Verify 2-module quiet zone is completely light (false)
	for r := 0; r < totalRows; r++ {
		for c := 0; c < totalCols; c++ {
			if r < qz || r >= qz+n || c < qz || c >= qz+n {
				if full[r][c] {
					return nil, fmt.Errorf("quiet zone dark at (%d,%d)", r, c)
				}
			}
		}
	}

	out := make([][]bool, n)
	for r := 0; r < n; r++ {
		out[r] = append([]bool(nil), full[qz+r][qz:qz+n]...)
	}
	return out, nil
}

func matricesEqual(a, b [][]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for r := range a {
		if len(a[r]) != len(b[r]) {
			return false
		}
		for c := range a[r] {
			if a[r][c] != b[r][c] {
				return false
			}
		}
	}
	return true
}

func TestDecodeQRPNGMatrixAndRenderDense1x2RoundTrip(t *testing.T) {
	defer ui.RestoreOverrides(ui.UIOverrides{})
	ui.RestoreOverrides(ui.UIOverrides{})

	cases := []struct {
		name          string
		n             int
		stride        int
		pad           [4]int
		transparentBg bool
	}{
		{"v1_21x21_no_border_1px", 21, 1, [4]int{0, 0, 0, 0}, false},
		{"v3_29x29_symmetric_5px", 29, 5, [4]int{15, 15, 15, 15}, false},
		{"v4_33x33_asymmetric_6px", 33, 6, [4]int{4, 12, 22, 9}, false},
		{"v4_33x33_transparent_alpha", 33, 4, [4]int{10, 10, 10, 10}, true},
		{"v6_41x41_8px", 41, 8, [4]int{16, 16, 16, 16}, false},
	}

	for idx, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := buildTestQRMatrix(tc.n, uint32(1000+idx*37))
			pngBytes := renderMatrixToPNG(t, want, tc.stride, tc.pad[0], tc.pad[1], tc.pad[2], tc.pad[3], tc.transparentBg)

			got, err := DecodeQRPNGMatrix(pngBytes)
			if err != nil {
				t.Fatalf("DecodeQRPNGMatrix failed: %v", err)
			}
			if !matricesEqual(got, want) {
				t.Fatalf("decoded %dx%d matrix mismatch", len(got), tc.n)
			}

			var buf bytes.Buffer
			plain := RenderQRMatrixDense1x2(&buf, got, false, false)
			if strings.Contains(plain, "\x1b") {
				t.Fatal("expected zero ANSI escapes when colorOn=false")
			}
			roundTrip, err := unpackDense1x2ToMatrix(plain, tc.n)
			if err != nil {
				t.Fatalf("unpackDense1x2ToMatrix failed: %v", err)
			}
			if !matricesEqual(roundTrip, want) {
				t.Fatal("Dense1x2 round-trip matrix mismatch")
			}

			colored := RenderQRMatrixDense1x2(&buf, got, true, false)
			if !strings.Contains(colored, "\x1b[") {
				t.Fatal("expected explicit ANSI color styling when colorOn=true")
			}
			if ansi.Strip(colored) != plain {
				t.Fatal("expected stripped colored Dense1x2 to match plain Dense1x2")
			}

			if asciiOut := RenderQRMatrixDense1x2(&buf, got, true, true); asciiOut != "" {
				t.Fatalf("expected empty string when asciiOnly=true, got %q", asciiOut)
			}
		})
	}

	// Non-integer scaled QR image (e.g. 33x33 stretched to 180x180 inside 200x200 PNG)
	t.Run("non_integer_scaled_200x200", func(t *testing.T) {
		want := buildTestQRMatrix(33, 4242)
		img := image.NewGray(image.Rect(0, 0, 200, 200))
		for y := 0; y < 200; y++ {
			for x := 0; x < 200; x++ {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
		for py := 0; py < 180; py++ {
			mr := py * 33 / 180
			for px := 0; px < 180; px++ {
				mc := px * 33 / 180
				if want[mr][mc] {
					img.SetGray(10+px, 10+py, color.Gray{Y: 0})
				}
			}
		}
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		got, err := DecodeQRPNGMatrix(b.Bytes())
		if err != nil {
			t.Fatalf("non-integer scaled decode failed: %v", err)
		}
		if !matricesEqual(got, want) {
			t.Fatal("non-integer scaled matrix mismatch")
		}
	})

	// Error paths: corrupt bytes, blank white PNG, solid black non-QR PNG, tiny image
	if _, err := DecodeQRPNGMatrix([]byte("not a png")); err == nil {
		t.Fatal("expected error for corrupt PNG")
	}
	blank := image.NewGray(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			blank.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	var blankBuf bytes.Buffer
	_ = png.Encode(&blankBuf, blank)
	if _, err := DecodeQRPNGMatrix(blankBuf.Bytes()); err == nil {
		t.Fatal("expected error for all-white blank PNG")
	}

	solidBlack := image.NewGray(image.Rect(0, 0, 64, 64))
	var blackBuf bytes.Buffer
	_ = png.Encode(&blackBuf, solidBlack)
	if _, err := DecodeQRPNGMatrix(blackBuf.Bytes()); err == nil {
		t.Fatal("expected error for solid-black non-QR PNG")
	}
}

func TestRunQRLoginEndToEndPollingStokenDeduplicationAndFileCleanup(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)
	t.Setenv("PANT_ASCII", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	defer ui.RestoreOverrides(ui.UIOverrides{})
	ui.SetNoColor(true)

	qrPNG := renderMatrixToPNG(t, buildTestQRMatrix(33, 777), 5, 10, 10, 10, 10, false)

	panSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "BAIDUID", Value: "BID_PREWARM_01:FG=1", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/disk/main":
			// Step 4 redirect target on pan.baidu.com sets Netdisk-specific STOKEN, SBOXTKN, SCRC
			http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "NETDISK_PAN_STOKEN_WIN", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "SBOXTKN", Value: "SBOX_VAL_1", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "SCRC", Value: "SCRC_VAL_1", Path: "/"})
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer panSrv.Close()

	var pollCalls atomic.Int32
	var passportSrv *httptest.Server
	passportSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/api/getqrcode":
			if gotTpl := r.URL.Query().Get("tpl"); gotTpl != "netdisk" {
				t.Errorf("getqrcode tpl=%q, want netdisk", gotTpl)
			}
			hostNoScheme := strings.TrimPrefix(passportSrv.URL, "http://")
			_, _ = fmt.Fprintf(w, `{"errno":0,"sign":"SIGN_HEX_88","imgurl":"%s/v2/api/qrcode?sign=SIGN_HEX_88"}`, hostNoScheme)
		case "/v2/api/qrcode":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(qrPNG)
		case "/channel/unicast":
			if r.URL.Query().Get("channel_id") != "SIGN_HEX_88" || r.URL.Query().Get("tpl") != "netdisk" {
				t.Errorf("unexpected unicast query: %s", r.URL.RawQuery)
			}
			n := pollCalls.Add(1)
			switch n {
			case 1:
				_, _ = io.WriteString(w, `{"errno":1}`)
			case 2, 3:
				_, _ = io.WriteString(w, `{"errno":0,"channel_v":"{\"status\":1}"}`)
			default:
				_, _ = io.WriteString(w, `{"errno":0,"channel_v":"{\"status\":0,\"v\":\"TICKET_V_88\"}"}`)
			}
		case "/v3/login/main/qrbdusslogin":
			if r.URL.Query().Get("bduss") != "TICKET_V_88" || r.URL.Query().Get("tpl") != "netdisk" {
				t.Errorf("unexpected qrbdusslogin query: %s", r.URL.RawQuery)
			}
			// Simulate Passport setting BDUSS, PTOKEN, AND a Passport-scoped STOKEN trap!
			http.SetCookie(w, &http.Cookie{Name: "BDUSS", Value: "REAL_BDUSS_88", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "BDUSS_BFESS", Value: "REAL_BDUSS_88", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "PTOKEN", Value: "REAL_PTOKEN_88", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "WRONG_PASSPORT_STOKEN", Path: "/"})
			_, _ = io.WriteString(w, `bd__cbs__qrlogin({'code':'110000','data':{'stokenList':"['netdisk#LIST_FALLBACK_STOKEN','tieba#other']"}})`)
		case "/v3/login/api/auth":
			http.Redirect(w, r, panSrv.URL+"/disk/main", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer passportSrv.Close()

	var errBuf bytes.Buffer
	var observedQRFile bool
	var observedPerm os.FileMode

	bduss, ptoken, stoken, cookies, err := runQRLoginWithOptions(context.Background(), qrLoginOptions{
		passportBaseURL: passportSrv.URL,
		panBaseURL:      panSrv.URL,
		totalTimeout:    5 * time.Second,
		pollTimeout:     2 * time.Second,
		pollInterval:    5 * time.Millisecond,
		errOut:          &errBuf,
		onQRReady: func(qrPath string) {
			if st, statErr := os.Stat(qrPath); statErr == nil {
				observedQRFile = true
				observedPerm = st.Mode().Perm()
			}
		},
	})
	if err != nil {
		t.Fatalf("runQRLoginWithOptions failed: %v", err)
	}
	if !observedQRFile {
		t.Fatal("expected qrcode.png to exist on disk while polling")
	}
	if runtime.GOOS != "windows" && observedPerm != 0600 {
		t.Fatalf("expected qrcode.png permissions 0600, got %04o", observedPerm)
	}
	if _, statErr := os.Stat(filepath.Join(cfgDir, qrCodeFileName)); !os.IsNotExist(statErr) {
		t.Fatalf("expected qrcode.png removed via defer after completion, statErr=%v", statErr)
	}

	if bduss != "REAL_BDUSS_88" || ptoken != "REAL_PTOKEN_88" || stoken != "NETDISK_PAN_STOKEN_WIN" {
		t.Fatalf("got bduss=%q ptoken=%q stoken=%q", bduss, ptoken, stoken)
	}
	if strings.Contains(cookies, "WRONG_PASSPORT_STOKEN") {
		t.Fatalf("Passport STOKEN leaked into deduplicated cookies: %q", cookies)
	}
	if count := strings.Count(cookies, "STOKEN="); count != 1 {
		t.Fatalf("expected exactly 1 STOKEN= in cookies, got %d in %q", count, cookies)
	}
	for _, wantSub := range []string{
		"BAIDUID=BID_PREWARM_01:FG=1",
		"BDUSS=REAL_BDUSS_88",
		"PTOKEN=REAL_PTOKEN_88",
		"STOKEN=NETDISK_PAN_STOKEN_WIN",
		"SBOXTKN=SBOX_VAL_1",
		"SCRC=SCRC_VAL_1",
	} {
		if !strings.Contains(cookies, wantSub) {
			t.Fatalf("cookies missing %q: %q", wantSub, cookies)
		}
	}

	// Verify "已扫码，请在手机上确认登录..." was printed exactly once despite two status=1 polls!
	if count := strings.Count(errBuf.String(), "已扫码，请在手机上确认登录"); count != 1 {
		t.Fatalf("expected scanned prompt printed once, got %d in:\n%s", count, errBuf.String())
	}
}

func TestRunQRLoginFallbacksCancelAndASCIIMode(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)
	defer ui.RestoreOverrides(ui.UIOverrides{})

	qrPNG := renderMatrixToPNG(t, buildTestQRMatrix(25, 999), 4, 8, 8, 8, 8, false)

	// 1. Step 4 SSO fails, Step 3 stokenList netdisk#<stoken> recovers Netdisk STOKEN cleanly; --ascii skips half-blocks.
	t.Run("stokenList_fallback_and_ascii_mode", func(t *testing.T) {
		ui.RestoreOverrides(ui.UIOverrides{NoColor: true, ASCII: true})

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				w.WriteHeader(http.StatusOK)
			case "/v2/api/getqrcode":
				_, _ = io.WriteString(w, `{"errno":0,"sign":"S1","imgurl":"/v2/api/qrcode?sign=S1"}`)
			case "/v2/api/qrcode":
				_, _ = w.Write(qrPNG)
			case "/channel/unicast":
				_, _ = io.WriteString(w, `{"errno":0,"channel_v":{"status":0,"v":"TK1"}}`)
			case "/v3/login/main/qrbdusslogin":
				http.SetCookie(w, &http.Cookie{Name: "BDUSS", Value: "BD_LIST", Path: "/"})
				http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "PASSPORT_ONLY_STOKEN", Path: "/"})
				_, _ = io.WriteString(w, `bd__cbs__qrlogin({"code":"110000","data":{"stokenList":"[&quot;netdisk#RECOVERED_FROM_LIST&quot;]"}})`)
			case "/v3/login/api/auth":
				w.WriteHeader(http.StatusBadGateway)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		var errBuf bytes.Buffer
		bduss, _, stoken, cookies, err := runQRLoginWithOptions(context.Background(), qrLoginOptions{
			passportBaseURL: srv.URL,
			panBaseURL:      srv.URL,
			totalTimeout:    3 * time.Second,
			pollInterval:    5 * time.Millisecond,
			errOut:          &errBuf,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bduss != "BD_LIST" || stoken != "RECOVERED_FROM_LIST" || !strings.Contains(cookies, "STOKEN=RECOVERED_FROM_LIST") {
			t.Fatalf("got bduss=%q stoken=%q cookies=%q", bduss, stoken, cookies)
		}
		if strings.ContainsAny(errBuf.String(), "█▀▄") {
			t.Fatalf("--ascii mode emitted Unicode half-blocks:\n%s", errBuf.String())
		}
		if !strings.Contains(errBuf.String(), filepath.Join(cfgDir, qrCodeFileName)) {
			t.Fatalf("--ascii mode did not print qrcode.png path:\n%s", errBuf.String())
		}
	})

	// 2. Neither Step 4 nor Step 3 provides Netdisk STOKEN -> omits Passport STOKEN and emits warning.
	t.Run("missing_netdisk_stoken_emits_warn", func(t *testing.T) {
		ui.RestoreOverrides(ui.UIOverrides{NoColor: true})

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				w.WriteHeader(http.StatusOK)
			case "/v2/api/getqrcode":
				_, _ = io.WriteString(w, `{"errno":0,"sign":"S2","imgurl":"/v2/api/qrcode?sign=S2"}`)
			case "/v2/api/qrcode":
				_, _ = w.Write(qrPNG)
			case "/channel/unicast":
				_, _ = io.WriteString(w, `{"errno":0,"channel_v":"{\"status\":0,\"v\":\"TK2\"}"}`)
			case "/v3/login/main/qrbdusslogin":
				http.SetCookie(w, &http.Cookie{Name: "BDUSS", Value: "BD_ONLY", Path: "/"})
				http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "PASSPORT_TRAP", Path: "/"})
				_, _ = io.WriteString(w, `bd__cbs__qrlogin({"code":"110000","data":{"stokenList":"[]"}})`)
			case "/v3/login/api/auth":
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
		defer srv.Close()

		var errBuf bytes.Buffer
		bduss, _, stoken, cookies, err := runQRLoginWithOptions(context.Background(), qrLoginOptions{
			passportBaseURL: srv.URL,
			panBaseURL:      srv.URL,
			totalTimeout:    3 * time.Second,
			pollInterval:    5 * time.Millisecond,
			errOut:          &errBuf,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bduss != "BD_ONLY" || stoken != "" || strings.Contains(cookies, "STOKEN=") {
			t.Fatalf("expected empty stoken and no Passport STOKEN in cookies, got stoken=%q cookies=%q", stoken, cookies)
		}
		if !strings.Contains(errBuf.String(), "STOKEN") {
			t.Fatalf("expected warning about missing STOKEN, got:\n%s", errBuf.String())
		}
	})

	// 3. Phone cancellation (status == 2) aborts immediately and cleans up qrcode.png.
	t.Run("phone_rejected_status_2", func(t *testing.T) {
		ui.RestoreOverrides(ui.UIOverrides{NoColor: true})

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				w.WriteHeader(http.StatusOK)
			case "/v2/api/getqrcode":
				_, _ = io.WriteString(w, `{"errno":0,"sign":"S3","imgurl":"/v2/api/qrcode?sign=S3"}`)
			case "/v2/api/qrcode":
				_, _ = w.Write(qrPNG)
			case "/channel/unicast":
				_, _ = io.WriteString(w, `{"errno":0,"channel_v":"{\"status\":2}"}`)
			}
		}))
		defer srv.Close()

		var errBuf bytes.Buffer
		_, _, _, _, err := runQRLoginWithOptions(context.Background(), qrLoginOptions{
			passportBaseURL: srv.URL,
			panBaseURL:      srv.URL,
			totalTimeout:    3 * time.Second,
			pollInterval:    5 * time.Millisecond,
			errOut:          &errBuf,
		})
		if err == nil || !strings.Contains(err.Error(), "已取消") {
			t.Fatalf("expected phone rejection error, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(cfgDir, qrCodeFileName)); !os.IsNotExist(statErr) {
			t.Fatalf("expected qrcode.png cleaned up on rejection, statErr=%v", statErr)
		}
	})

	// 4. Context cancellation (Ctrl+C simulation) aborts blocked 30s long-poll immediately and cleans up qrcode.png.
	t.Run("ctrl_c_cancels_long_poll_and_removes_qr_png", func(t *testing.T) {
		ui.RestoreOverrides(ui.UIOverrides{NoColor: true})

		pollEntered := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				w.WriteHeader(http.StatusOK)
			case "/v2/api/getqrcode":
				_, _ = io.WriteString(w, `{"errno":0,"sign":"S4","imgurl":"/v2/api/qrcode?sign=S4"}`)
			case "/v2/api/qrcode":
				_, _ = w.Write(qrPNG)
			case "/channel/unicast":
				select {
				case <-pollEntered:
				default:
					close(pollEntered)
				}
				<-r.Context().Done()
			}
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			<-pollEntered
			time.Sleep(15 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		var errBuf bytes.Buffer
		_, _, _, _, err := runQRLoginWithOptions(ctx, qrLoginOptions{
			passportBaseURL: srv.URL,
			panBaseURL:      srv.URL,
			totalTimeout:    30 * time.Second,
			pollTimeout:     30 * time.Second,
			pollInterval:    5 * time.Millisecond,
			errOut:          &errBuf,
		})
		elapsed := time.Since(start)
		if !errors.Is(err, ErrLoginCanceled) {
			t.Fatalf("expected ErrLoginCanceled, got %v", err)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("long-poll took too long to abort after cancel: %v", elapsed)
		}
		if _, statErr := os.Stat(filepath.Join(cfgDir, qrCodeFileName)); !os.IsNotExist(statErr) {
			t.Fatalf("expected qrcode.png deleted on Ctrl+C cancel, statErr=%v", statErr)
		}
	})
}

func TestQRDecodeOuterFrameCenterIconAndAuthDeletedCookieAndLongCookiePaste(t *testing.T) {
	defer ui.RestoreOverrides(ui.UIOverrides{})
	ui.RestoreOverrides(ui.UIOverrides{NoColor: true})

	// 1. QR PNG with a 2px dark outer frame border, 14px quiet zone, rounded finder corners,
	//    gray compression noise, AND a 5x5 center brand icon must still decode to the exact 33x33 grid.
	t.Run("outer_frame_rounded_corners_and_center_icon", func(t *testing.T) {
		const n = 33
		const stride = 6
		const pad = 16
		want := buildTestQRMatrix(n, 20261118)
		// Embed a 5x5 brand icon pattern in the center [14..18, 14..18]
		for r := 14; r <= 18; r++ {
			for c := 14; c <= 18; c++ {
				want[r][c] = (r == 14 || r == 18 || c == 14 || c == 18 || (r == 16 && c == 16))
			}
		}

		w := 2*pad + n*stride
		h := 2*pad + n*stride
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				// Noisy off-white background (#F2F2F2) + 2px dark outer frame (#222222)
				if x < 2 || x >= w-2 || y < 2 || y >= h-2 {
					img.SetNRGBA(x, y, color.NRGBA{R: 34, G: 34, B: 34, A: 255})
				} else {
					img.SetNRGBA(x, y, color.NRGBA{R: 242, G: 242, B: 242, A: 255})
				}
			}
		}
		for r := 0; r < n; r++ {
			for c := 0; c < n; c++ {
				if !want[r][c] {
					continue
				}
				fg := color.NRGBA{R: 24, G: 24, B: 24, A: 255}
				if r >= 14 && r <= 18 && c >= 14 && c <= 18 {
					// Baidu brand blue icon (#2932E1)
					fg = color.NRGBA{R: 0x29, G: 0x32, B: 0xE1, A: 255}
				}
				for dy := 0; dy < stride; dy++ {
					for dx := 0; dx < stride; dx++ {
						img.SetNRGBA(pad+c*stride+dx, pad+r*stride+dy, fg)
					}
				}
			}
		}
		// Shave 2x2 outer corner pixels off top-left finder to simulate rounded finder corners
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				img.SetNRGBA(pad+dx, pad+dy, color.NRGBA{R: 242, G: 242, B: 242, A: 255})
			}
		}

		var pngBuf bytes.Buffer
		if err := png.Encode(&pngBuf, img); err != nil {
			t.Fatal(err)
		}
		got, err := DecodeQRPNGMatrix(pngBuf.Bytes())
		if err != nil {
			t.Fatalf("DecodeQRPNGMatrix with outer frame + center icon failed: %v", err)
		}
		if !matricesEqual(got, want) {
			t.Fatal("decoded matrix mismatch on framed/icon QR PNG")
		}
	})

	// 2. Step 4 `STOKEN=deleted; Max-Age=0` and Passport `.baidu.com` STOKEN echo must NOT overwrite real Netdisk STOKEN,
	//    and URL-encoded `stokenList` (`netdisk%23...%22`) must not swallow `%22`.
	t.Run("deleted_cookie_guard_and_percent_encoded_stoken_list", func(t *testing.T) {
		if got := extractNetdiskStokenFromBody([]byte(`{"stokenList":"%5B%22netdisk%23ENC_STOKEN_01%22%5D"}`)); got != "ENC_STOKEN_01" {
			t.Fatalf("extractNetdiskStokenFromBody percent-encoded = %q, want ENC_STOKEN_01", got)
		}
		if got := extractNetdiskStokenFromBody([]byte(`{"stokenList":"netdisk#TRAIL_QUOTE%22"}`)); got != "TRAIL_QUOTE" {
			t.Fatalf("extractNetdiskStokenFromBody swallowed %%22: got %q, want TRAIL_QUOTE", got)
		}

		cfgDir := t.TempDir()
		t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)
		qrPNG := renderMatrixToPNG(t, buildTestQRMatrix(25, 31415), 4, 8, 8, 8, 8, false)

		panSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				w.WriteHeader(http.StatusOK)
			case "/disk/main":
				// Real Netdisk STOKEN followed by PHP-style .baidu.com deletion cookie AND Passport STOKEN echo
				http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "REAL_NETDISK_STOKEN_99", Path: "/"})
				http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "deleted", Path: "/", Domain: ".baidu.com", MaxAge: -1, Expires: time.Unix(1, 0)})
				http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "PASSPORT_ECHO_STOKEN", Path: "/", Domain: ".baidu.com"})
				w.WriteHeader(http.StatusOK)
			default:
				http.NotFound(w, r)
			}
		}))
		defer panSrv.Close()

		passportSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/api/getqrcode":
				_, _ = io.WriteString(w, `{"errno":0,"sign":"S_DEL","imgurl":"/v2/api/qrcode?sign=S_DEL"}`)
			case "/v2/api/qrcode":
				_, _ = w.Write(qrPNG)
			case "/channel/unicast":
				_, _ = io.WriteString(w, `{"errno":0,"channel_v":{"status":0,"v":"TK_DEL"}}`)
			case "/v3/login/main/qrbdusslogin":
				http.SetCookie(w, &http.Cookie{Name: "BDUSS", Value: "BD_DEL_99", Path: "/"})
				http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "PASSPORT_ECHO_STOKEN", Path: "/"})
				_, _ = io.WriteString(w, `bd__cbs__qrlogin({"code":"110000","data":{}})`)
			case "/v3/login/api/auth":
				http.Redirect(w, r, panSrv.URL+"/disk/main", http.StatusFound)
			}
		}))
		defer passportSrv.Close()

		var errBuf bytes.Buffer
		_, _, stoken, cookies, err := runQRLoginWithOptions(context.Background(), qrLoginOptions{
			passportBaseURL: passportSrv.URL,
			panBaseURL:      panSrv.URL,
			totalTimeout:    3 * time.Second,
			pollInterval:    5 * time.Millisecond,
			errOut:          &errBuf,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stoken != "REAL_NETDISK_STOKEN_99" || !strings.Contains(cookies, "STOKEN=REAL_NETDISK_STOKEN_99") {
			t.Fatalf("deleted/echo cookie poisoned Netdisk STOKEN: stoken=%q cookies=%q", stoken, cookies)
		}
	})

	// 3. >8KB Cookie paste MUST NOT truncate past 4096 bytes, and multi-line BDUSS+STOKEN paste MUST NOT lose STOKEN.
	t.Run("long_8kb_cookie_and_multiline_bduss_stoken_prompts", func(t *testing.T) {
		longPad := strings.Repeat("x", 6000)
		rawCookieLine := "Cookie: AB_SR=" + longPad + "; BAIDUID=BID_8K; BDUSS=BDUSS_AT_TAIL_8K; STOKEN=STOKEN_AT_TAIL_8K\n"
		gotCookies, err := runCookieLoginPromptFrom(strings.NewReader(rawCookieLine))
		if err != nil {
			t.Fatalf("runCookieLoginPromptFrom >8KB failed: %v", err)
		}
		if !strings.Contains(gotCookies, "BDUSS=BDUSS_AT_TAIL_8K") || !strings.Contains(gotCookies, "STOKEN=STOKEN_AT_TAIL_8K") {
			t.Fatalf(">8KB Cookie line was truncated! len=%d tail=%q", len(gotCookies), gotCookies[len(gotCookies)-64:])
		}

		gotBDUSS, gotSTOKEN, err := runBDUSSLoginPromptFrom(strings.NewReader("pasted_bduss_line1\r\npasted_stoken_line2\r\n"))
		if err != nil {
			t.Fatalf("runBDUSSLoginPromptFrom multi-line failed: %v", err)
		}
		if gotBDUSS != "pasted_bduss_line1" || gotSTOKEN != "pasted_stoken_line2" {
			t.Fatalf("multi-line paste lost second line: bduss=%q stoken=%q", gotBDUSS, gotSTOKEN)
		}
	})
}

func TestRunQRLoginTransientEOFRetryAndCertFailureNoRetry(t *testing.T) {
	defer ui.RestoreOverrides(ui.UIOverrides{})
	ui.RestoreOverrides(ui.UIOverrides{NoColor: true})

	hijackCloseImmediately := func(t *testing.T, w http.ResponseWriter) {
		t.Helper()
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("ResponseWriter does not support http.Hijacker")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("Hijack failed: %v", err)
		}
		_ = conn.Close()
	}

	hijackTruncatedBody := func(t *testing.T, w http.ResponseWriter) {
		t.Helper()
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("ResponseWriter does not support http.Hijacker")
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			t.Fatalf("Hijack failed: %v", err)
		}
		_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: image/png\r\nContent-Length: 65536\r\n\r\n\x89PNG")
		_ = rw.Flush()
		_ = conn.Close()
	}

	// 1. Transient bare EOF / unexpected EOF across prewarm, getqrcode, qrcode PNG, unicast, qrbdusslogin, and auth SSO recovers cleanly.
	t.Run("transient_eof_recovery_across_all_qr_steps", func(t *testing.T) {
		cfgDir := t.TempDir()
		t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)
		qrPNG := renderMatrixToPNG(t, buildTestQRMatrix(29, 8080), 4, 8, 8, 8, 8, false)

		var prewarmHits, getQRHits, pngHits, pollHits, loginHits, authHits atomic.Int32

		panSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				if prewarmHits.Add(1) == 1 {
					hijackCloseImmediately(t, w)
					return
				}
				http.SetCookie(w, &http.Cookie{Name: "BAIDUID", Value: "BID_AFTER_EOF_01:FG=1", Path: "/"})
				w.WriteHeader(http.StatusOK)
			case "/disk/main":
				http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "NETDISK_EOF_RECOVERED_STOKEN", Path: "/"})
				w.WriteHeader(http.StatusOK)
			default:
				http.NotFound(w, r)
			}
		}))
		defer panSrv.Close()

		passportSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/api/getqrcode":
				if getQRHits.Add(1) <= 2 {
					hijackCloseImmediately(t, w)
					return
				}
				_, _ = io.WriteString(w, `{"errno":0,"sign":"S_EOF","imgurl":"/v2/api/qrcode?sign=S_EOF"}`)
			case "/v2/api/qrcode":
				if pngHits.Add(1) == 1 {
					hijackTruncatedBody(t, w)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(qrPNG)
			case "/channel/unicast":
				if pollHits.Add(1) == 1 {
					hijackCloseImmediately(t, w)
					return
				}
				_, _ = io.WriteString(w, `{"errno":0,"channel_v":{"status":0,"v":"TK_EOF_OK"}}`)
			case "/v3/login/main/qrbdusslogin":
				if loginHits.Add(1) == 1 {
					hijackCloseImmediately(t, w)
					return
				}
				http.SetCookie(w, &http.Cookie{Name: "BDUSS", Value: "BDUSS_EOF_OK", Path: "/"})
				_, _ = io.WriteString(w, `bd__cbs__qrlogin({"code":"110000","data":{}})`)
			case "/v3/login/api/auth":
				if authHits.Add(1) == 1 {
					hijackCloseImmediately(t, w)
					return
				}
				http.Redirect(w, r, panSrv.URL+"/disk/main", http.StatusFound)
			default:
				http.NotFound(w, r)
			}
		}))
		defer passportSrv.Close()

		var errBuf bytes.Buffer
		bduss, _, stoken, cookies, err := runQRLoginWithOptions(context.Background(), qrLoginOptions{
			passportBaseURL: passportSrv.URL,
			panBaseURL:      panSrv.URL,
			totalTimeout:    10 * time.Second,
			pollTimeout:     2 * time.Second,
			pollInterval:    5 * time.Millisecond,
			errOut:          &errBuf,
		})
		if err != nil {
			t.Fatalf("expected transient EOF recovery to succeed, got error: %v", err)
		}
		if getQRHits.Load() != 3 || pngHits.Load() != 2 || pollHits.Load() != 2 || loginHits.Load() != 2 || authHits.Load() != 2 {
			t.Fatalf("unexpected retry counts: prewarm=%d getqr=%d png=%d poll=%d login=%d auth=%d",
				prewarmHits.Load(), getQRHits.Load(), pngHits.Load(), pollHits.Load(), loginHits.Load(), authHits.Load())
		}
		if bduss != "BDUSS_EOF_OK" || stoken != "NETDISK_EOF_RECOVERED_STOKEN" {
			t.Fatalf("got bduss=%q stoken=%q", bduss, stoken)
		}
		if !strings.Contains(cookies, "BAIDUID=BID_AFTER_EOF_01:FG=1") || !strings.Contains(cookies, "STOKEN=NETDISK_EOF_RECOVERED_STOKEN") {
			t.Fatalf("missing recovered cookies in %q", cookies)
		}
	})

	// 2. Untrusted TLS certificate on Passport MUST fail immediately on attempt 1 without wasting retries.
	t.Run("untrusted_tls_cert_is_not_retried", func(t *testing.T) {
		cfgDir := t.TempDir()
		t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)

		panSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer panSrv.Close()

		var tlsHandshakes atomic.Int32
		untrustedPassport := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"errno":0,"sign":"S_MITM","imgurl":"/v2/api/qrcode"}`)
		}))
		untrustedPassport.Config.ErrorLog = log.New(io.Discard, "", 0)
		untrustedPassport.TLS = &tls.Config{
			GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
				tlsHandshakes.Add(1)
				return nil, nil
			},
		}
		untrustedPassport.StartTLS()
		defer untrustedPassport.Close()

		var errBuf bytes.Buffer
		_, _, _, _, err := runQRLoginWithOptions(context.Background(), qrLoginOptions{
			passportBaseURL: untrustedPassport.URL,
			panBaseURL:      panSrv.URL,
			totalTimeout:    5 * time.Second,
			pollInterval:    5 * time.Millisecond,
			errOut:          &errBuf,
		})
		if err == nil {
			t.Fatal("expected untrusted self-signed TLS server to be rejected")
		}
		if tlsHandshakes.Load() != 1 {
			t.Fatalf("expected untrusted TLS cert failure NOT to retry, got %d TLS handshakes", tlsHandshakes.Load())
		}
	})

	// 3. Context cancellation during EOF retry backoff aborts immediately with ErrLoginCanceled.
	t.Run("cancel_during_eof_retry_backoff", func(t *testing.T) {
		cfgDir := t.TempDir()
		t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		panSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer panSrv.Close()

		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) == 1 {
				go func() {
					time.Sleep(20 * time.Millisecond)
					cancel()
				}()
			}
			hijackCloseImmediately(t, w)
		}))
		defer srv.Close()

		var errBuf bytes.Buffer
		_, _, _, _, err := runQRLoginWithOptions(ctx, qrLoginOptions{
			passportBaseURL: srv.URL,
			panBaseURL:      panSrv.URL,
			totalTimeout:    5 * time.Second,
			errOut:          &errBuf,
		})
		if !errors.Is(err, ErrLoginCanceled) {
			t.Fatalf("expected ErrLoginCanceled during retry backoff, got %v", err)
		}
		if hits.Load() != 1 {
			t.Fatalf("expected retry loop to abort during first backoff without firing attempt #2, got %d hits", hits.Load())
		}
	})

	// 4. Gateway 502/503/504 on getqrcode / qrcode PNG recovers via retry, while HTTP 404/500 fails fast without retry;
	//    and normalizeQRImgURL upgrades http://passport.baidu.com/... to https:// when passportBase is https://.
	t.Run("gateway_502_503_504_retried_and_404_not_retried_and_https_upgrade", func(t *testing.T) {
		if got := normalizeQRImgURL("http://passport.baidu.com/v2/api/qrcode?sign=abc", "https://passport.baidu.com"); got != "https://passport.baidu.com/v2/api/qrcode?sign=abc" {
			t.Fatalf("normalizeQRImgURL failed to upgrade http->https for baidu.com: got %q", got)
		}

		cfgDir := t.TempDir()
		t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)
		qrPNG := renderMatrixToPNG(t, buildTestQRMatrix(25, 404502), 4, 8, 8, 8, 8, false)

		var getQRHits, pngHits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				w.WriteHeader(http.StatusOK)
			case "/v2/api/getqrcode":
				switch getQRHits.Add(1) {
				case 1:
					w.WriteHeader(http.StatusBadGateway)
					return
				case 2:
					w.WriteHeader(http.StatusGatewayTimeout)
					return
				}
				_, _ = io.WriteString(w, `{"errno":0,"sign":"S_502","imgurl":"/v2/api/qrcode?sign=S_502"}`)
			case "/v2/api/qrcode":
				if pngHits.Add(1) == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(qrPNG)
			case "/channel/unicast":
				_, _ = io.WriteString(w, `{"errno":0,"channel_v":{"status":0,"v":"TK_502"}}`)
			case "/v3/login/main/qrbdusslogin":
				http.SetCookie(w, &http.Cookie{Name: "BDUSS", Value: "BD_502_OK", Path: "/"})
				_, _ = io.WriteString(w, `bd__cbs__qrlogin({"code":"110000","data":{"stokenList":"['netdisk#ST_502_OK']"}})`)
			case "/v3/login/api/auth":
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer srv.Close()

		var errBuf bytes.Buffer
		bduss, _, stoken, _, err := runQRLoginWithOptions(context.Background(), qrLoginOptions{
			passportBaseURL: srv.URL,
			panBaseURL:      srv.URL,
			totalTimeout:    8 * time.Second,
			pollTimeout:     2 * time.Second,
			pollInterval:    5 * time.Millisecond,
			errOut:          &errBuf,
		})
		if err != nil {
			t.Fatalf("expected 502/503/504 recovery to succeed, got: %v", err)
		}
		if getQRHits.Load() != 3 || pngHits.Load() != 2 || bduss != "BD_502_OK" || stoken != "ST_502_OK" {
			t.Fatalf("unexpected 5xx retry result: getqr=%d png=%d bduss=%q stoken=%q", getQRHits.Load(), pngHits.Load(), bduss, stoken)
		}

		var hits404 atomic.Int32
		srv404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits404.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv404.Close()
		_, _, _, _, err = runQRLoginWithOptions(context.Background(), qrLoginOptions{
			passportBaseURL: srv404.URL,
			panBaseURL:      srv.URL,
			totalTimeout:    5 * time.Second,
			errOut:          &errBuf,
		})
		if err == nil || hits404.Load() != 1 {
			t.Fatalf("expected HTTP 404 to fail fast on attempt 1, got hits=%d err=%v", hits404.Load(), err)
		}
	})

	// 5. Transport timeout (e.g. TLSHandshakeTimeout / ResponseHeaderTimeout) during pollQRUnicast MUST rotate host IP
	//    rather than being swallowed as a normal 40s long-poll DeadlineExceeded.
	t.Run("poll_unicast_transport_timeout_rotates_host_cache", func(t *testing.T) {
		const pollHost = "unicast-timeout-rotate.baidu.internal"
		requester.SetTCPHostBind(pollHost, "")
		defer requester.InvalidateTCPHostCache(pollHost)

		client := requester.NewHTTPClient()
		client.SetResponseHeaderTimeout(40 * time.Millisecond)
		defer client.CloseIdleConnections()

		var pollAttempts atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pollAttempts.Add(1) == 1 {
				// Exceed client's 40ms ResponseHeaderTimeout while pollTimeout is 2s!
				select {
				case <-time.After(150 * time.Millisecond):
				case <-r.Context().Done():
				}
				return
			}
			_, _ = io.WriteString(w, `{"errno":0,"channel_v":{"status":0,"v":"TK_AFTER_TRANSPORT_TIMEOUT"}}`)
		}))
		defer srv.Close()

		var errBuf bytes.Buffer
		ticket, err := pollQRUnicast(
			context.Background(),
			context.Background(),
			client,
			srv.URL,
			srv.URL,
			"SIGN_TO",
			"GID_TO",
			2*time.Second,
			5*time.Millisecond,
			&errBuf,
		)
		if err != nil || ticket != "TK_AFTER_TRANSPORT_TIMEOUT" {
			t.Fatalf("pollQRUnicast failed after transport timeout: ticket=%q err=%v", ticket, err)
		}

		// Also verify repeated transport timeouts hit consecutiveNetErrs >= 5 instead of looping forever as consecutiveNetErrs=0!
		alwaysStall := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(150 * time.Millisecond):
			case <-r.Context().Done():
			}
		}))
		defer alwaysStall.Close()

		totalCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, stallErr := pollQRUnicast(
			context.Background(),
			totalCtx,
			client,
			alwaysStall.URL,
			alwaysStall.URL,
			"SIGN_STALL",
			"GID_STALL",
			2*time.Second,
			5*time.Millisecond,
			&errBuf,
		)
		if stallErr == nil || !strings.Contains(stallErr.Error(), "扫码状态轮询网络异常") {
			t.Fatalf("expected repeated transport timeouts to trigger consecutiveNetErrs failure, got: %v", stallErr)
		}
	})
}
