package pcscommand

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
)

type mockRoundTripper func(*http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func jsonHTTPResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func setupMockCommandEnv(t *testing.T, handler func(*http.Request) (*http.Response, error)) {
	t.Helper()
	cfgDir := t.TempDir()
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)
	if err := pcsconfig.Config.Init(); err != nil {
		t.Fatalf("Config.Init failed: %v", err)
	}
	t.Cleanup(func() {
		_ = pcsconfig.Config.Close()
		ui.RestoreOverrides(ui.UIOverrides{})
	})

	pcsconfig.Config.SaveDir = filepath.Join(cfgDir, "Downloads")
	pcsconfig.Config.BaiduActiveUID = 777001
	pcsconfig.Config.BaiduUserList = pcsconfig.BaiduUserList{
		{
			BaiduBase: pcsconfig.BaiduBase{UID: 777001, Name: "tester"},
			BDUSS:     "mock_bduss",
			STOKEN:    "mock_stoken",
			Workdir:   "/work",
		},
	}
	if _, err := pcsconfig.Config.SwitchUser(&pcsconfig.BaiduBase{UID: 777001}); err != nil {
		t.Fatalf("SwitchUser failed: %v", err)
	}
	pcs := pcsconfig.Config.ActiveUserBaiduPCS()
	pcs.GetClient().Transport = mockRoundTripper(handler)
}

func captureOutputs(t *testing.T, fn func() error) (stdout, stderr string, err error) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	os.Stdout, os.Stderr = outW, errW

	runErr := fn()

	_ = outW.Close()
	_ = errW.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	var outB, errB bytes.Buffer
	_, _ = io.Copy(&outB, outR)
	_, _ = io.Copy(&errB, errR)
	return outB.String(), errB.String(), runErr
}

func TestRunDownloadNonexistentRemotePathReturnsErrorWithoutFalseSuccess(t *testing.T) {
	setupMockCommandEnv(t, func(req *http.Request) (*http.Response, error) {
		return jsonHTTPResponse(`{"error_code":31066,"error_msg":"file does not exist"}`), nil
	})

	stdout, _, err := captureOutputs(t, func() error {
		return RunDownload([]string{"/nonexistent.zip"}, &DownloadOptions{DisableWildcard: true})
	})
	if err == nil {
		t.Fatal("expected RunDownload on nonexistent remote path (31066) to return non-nil error")
	}
	if !strings.Contains(err.Error(), "31066") {
		t.Fatalf("expected error to preserve remote code 31066, got: %v", err)
	}
	if strings.Contains(stdout, "下载完成") || strings.Contains(stdout, "下载队列就绪") {
		t.Fatalf("nonexistent download must not print queue/completion success to stdout, got:\n%s", stdout)
	}
}

func TestRunDownloadDisableWildcardSkipsShellPatternListScan(t *testing.T) {
	var listScanCalled bool
	setupMockCommandEnv(t, func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/api/list") {
			listScanCalled = true
			return jsonHTTPResponse(`{"errno":0,"list":[]}`), nil
		}
		if req.URL.Query().Get("method") == "meta" {
			// Return a 0-byte file whose name contains '[' and '*' so DownloadTaskUnit creates empty local file without network dlink.
			return jsonHTTPResponse(`{"error_code":0,"list":[{"fs_id":501,"path":"/work/[1080p] star*.mkv","server_filename":"[1080p] star*.mkv","size":0,"isdir":0}]}`), nil
		}
		return jsonHTTPResponse(`{"error_code":0}`), nil
	})

	saveDir := t.TempDir()
	_, _, err := captureOutputs(t, func() error {
		return RunDownload([]string{"/work/[1080p] star*.mkv"}, &DownloadOptions{
			DisableWildcard: true,
			SaveTo:          saveDir,
		})
	})
	if err != nil {
		t.Fatalf("RunDownload with DisableWildcard failed on bracket/star filename: %v", err)
	}
	if listScanCalled {
		t.Fatal("DisableWildcard=true should not trigger /api/list shell pattern scan")
	}
	if _, statErr := os.Stat(filepath.Join(saveDir, "[1080p] star*.mkv")); statErr != nil {
		t.Fatalf("expected literal file created locally, stat err: %v", statErr)
	}
}

func TestTreeDepthZeroDepthOneAndTrailingNilBranch(t *testing.T) {
	setupMockCommandEnv(t, func(req *http.Request) (*http.Response, error) {
		dir := req.URL.Query().Get("dir")
		switch dir {
		case "/work":
			return jsonHTTPResponse(`{"errno":0,"list":[{"fs_id":1,"path":"/work/sub","server_filename":"sub","isdir":1},{"fs_id":2,"path":"/work/root.txt","server_filename":"root.txt","size":12,"isdir":0},null]}`), nil
		case "/work/sub":
			return jsonHTTPResponse(`{"errno":0,"list":[{"fs_id":3,"path":"/work/sub/child.txt","server_filename":"child.txt","size":24,"isdir":0}]}`), nil
		default:
			return jsonHTTPResponse(`{"errno":0,"list":[]}`), nil
		}
	})

	ui.SetNoColor(true)
	ui.SetASCIIOnly(true)

	// Depth == 0: print root only!
	out0, _, err := captureOutputs(t, func() error {
		return RunTree("/work", 0, &TreeOptions{Depth: 0})
	})
	if err != nil {
		t.Fatalf("RunTree depth=0 failed: %v", err)
	}
	if strings.TrimSpace(out0) != "/work/" {
		t.Fatalf("expected depth=0 to print only '/work/', got:\n%s", out0)
	}

	// Depth == 1: print root + direct children (sub/ and root.txt), NOT child.txt, and trailing null must not break `-- on root.txt!
	out1, _, err := captureOutputs(t, func() error {
		return RunTree("/work", 0, &TreeOptions{Depth: 1})
	})
	if err != nil {
		t.Fatalf("RunTree depth=1 failed: %v", err)
	}
	if strings.Contains(out1, "child.txt") {
		t.Fatalf("depth=1 recursed one level too deep (printed child.txt):\n%s", out1)
	}
	if !strings.Contains(out1, "`-- root.txt") {
		t.Fatalf("trailing nil entry broke last-branch connector (`--):\n%s", out1)
	}
}

func TestParseShareTransferArgsAndCollectDownloadPaths(t *testing.T) {
	cases := []struct {
		name     string
		params   []string
		wantLink string
		wantFeat string
		wantPwd  string
		wantErr  bool
	}{
		{
			name:     "schemeless_two_arg",
			params:   []string{"pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ", "704e"},
			wantLink: "https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ",
			wantFeat: "1VYzSl7465sdrQXe8GT5RdQ",
			wantPwd:  "704e",
		},
		{
			name:     "password_free_public_share",
			params:   []string{"pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ"},
			wantLink: "https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ",
			wantFeat: "1VYzSl7465sdrQXe8GT5RdQ",
			wantPwd:  "",
		},
		{
			name:     "single_arg_chinese_share_text_fullwidth_colon",
			params:   []string{"通过百度网盘分享的文件：资料包 链接：https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ 提取码：ab12 复制这段内容打开"},
			wantLink: "https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ",
			wantFeat: "1VYzSl7465sdrQXe8GT5RdQ",
			wantPwd:  "ab12",
		},
		{
			name:     "unquoted_multi_arg_share_text_ascii_colon",
			params:   []string{"链接:", "pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ", "提取码:", "9xYz", "--来自百度网盘"},
			wantLink: "https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ",
			wantFeat: "1VYzSl7465sdrQXe8GT5RdQ",
			wantPwd:  "9xYz",
		},
		{
			name:     "init_surl_format",
			params:   []string{"https://pan.baidu.com/share/init?surl=VYzSl7465sdrQXe8GT5RdQ", "提取码:1234"},
			wantLink: "https://pan.baidu.com/share/init?surl=VYzSl7465sdrQXe8GT5RdQ",
			wantFeat: "1VYzSl7465sdrQXe8GT5RdQ",
			wantPwd:  "1234",
		},
		{
			name:     "adjacent_chinese_after_pwd_no_space",
			params:   []string{"链接: https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ?pwd=704e提取码: 704e 复制这段内容"},
			wantLink: "https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ?pwd=704e",
			wantFeat: "1VYzSl7465sdrQXe8GT5RdQ",
			wantPwd:  "704e",
		},
		{
			name:     "adjacent_fullwidth_comma_after_surl",
			params:   []string{"https://pan.baidu.com/share/init?surl=VYzSl7465sdrQXe8GT5RdQ，提取码：704e"},
			wantLink: "https://pan.baidu.com/share/init?surl=VYzSl7465sdrQXe8GT5RdQ",
			wantFeat: "1VYzSl7465sdrQXe8GT5RdQ",
			wantPwd:  "704e",
		},
		{
			name:    "reject_bdlink",
			params:  []string{"https://pan.baidu.com/#bdlink=abcdef"},
			wantErr: true,
		},
		{
			name:    "reject_bad_pwd_len",
			params:  []string{"https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ", "12345"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotLink, gotFeat, gotPwd, err := parseShareTransferArgs(tc.params)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got (%q, %q, %q)", gotLink, gotFeat, gotPwd)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotLink != tc.wantLink || gotFeat != tc.wantFeat || gotPwd != tc.wantPwd {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)", gotLink, gotFeat, gotPwd, tc.wantLink, tc.wantFeat, tc.wantPwd)
			}
		})
	}

	dlPaths := buildTransferDownloadPaths("/work/archive等文件", []string{"a.zip", "b.pdf"})
	if len(dlPaths) != 2 || dlPaths[0] != "/work/archive等文件/a.zip" || dlPaths[1] != "/work/archive等文件/b.pdf" {
		t.Fatalf("unexpected --collect --download target paths: %#v", dlPaths)
	}
}

func TestQuietSuppressionASCIIAndStderrHygiene(t *testing.T) {
	setupMockCommandEnv(t, func(req *http.Request) (*http.Response, error) {
		p := req.URL.Path
		m := req.URL.Query().Get("method")
		switch {
		case strings.Contains(p, "/api/quota"):
			return jsonHTTPResponse(`{"errno":0,"total":1099511627776,"used":549755813888}`), nil
		case strings.Contains(p, "/api/list"):
			return jsonHTTPResponse(`{"errno":0,"list":[{"fs_id":10,"path":"/work/report.pdf","server_filename":"report.pdf","size":1024,"isdir":0}]}`), nil
		case m == "mkdir":
			return jsonHTTPResponse(`{"error_code":0,"path":"/work/newdir"}`), nil
		case m == "locatedownload":
			return jsonHTTPResponse(`{"error_code":31066,"error_msg":"file does not exist"}`), nil
		case strings.Contains(p, "/share/record"):
			return jsonHTTPResponse(`{"errno":0,"list":[{"shareId":991,"shortlink":"https://pan.baidu.com/s/1abc","typicalPath":"/work/report.pdf","public":0,"status":0}]}`), nil
		case strings.Contains(p, "/share/surlinfoinrecord"):
			return jsonHTTPResponse(`{"errno":-9}`), nil
		default:
			return jsonHTTPResponse(`{"error_code":0,"errno":0}`), nil
		}
	})

	ui.SetNoColor(true)
	ui.SetASCIIOnly(true)

	// 1. ls with --ascii (not quiet): summary uses "SUM" instead of "Σ" (#22).
	lsASCIIOut, _, err := captureOutputs(t, func() error {
		return RunLs("/work", nil, baidupcs.DefaultOrderOptions)
	})
	if err != nil {
		t.Fatalf("RunLs failed: %v", err)
	}
	if strings.Contains(lsASCIIOut, "Σ") || !strings.Contains(lsASCIIOut, "SUM") {
		t.Fatalf("expected ASCII 'SUM' instead of 'Σ' in RunLs --ascii output:\n%s", lsASCIIOut)
	}

	// 2. --quiet: quota preserves primary data; ls preserves table while hiding decorative header/footer; mkdir suppresses chatter (#21).
	ui.SetQuiet(true)
	quotaOut, _, err := captureOutputs(t, func() error {
		return RunGetQuota()
	})
	if err != nil || strings.TrimSpace(quotaOut) == "" {
		t.Fatalf("expected 'pant -q quota' to preserve primary quota output, got %q (err=%v)", quotaOut, err)
	}

	lsQuietOut, _, err := captureOutputs(t, func() error {
		return RunLs("/work", nil, baidupcs.DefaultOrderOptions)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lsQuietOut, "report.pdf") || strings.Contains(lsQuietOut, "当前目录:") || strings.Contains(lsQuietOut, "SUM") {
		t.Fatalf("expected 'pant -q ls' to print table without decorative chatter, got:\n%s", lsQuietOut)
	}

	mkdirQuietOut, _, err := captureOutputs(t, func() error {
		return RunMkdir("newdir")
	})
	if err != nil || mkdirQuietOut != "" {
		t.Fatalf("expected 'pant -q mkdir' to produce empty stdout, got %q (err=%v)", mkdirQuietOut, err)
	}

	// 3. locate when all paths fail must NOT pollute stdout with UA hint or double-print to stderr (#20).
	ui.SetQuiet(false)
	locOut, locErr, err := captureOutputs(t, func() error {
		return RunLocateDownload([]string{"/work/missing.bin"}, nil)
	})
	if err == nil {
		t.Fatal("expected RunLocateDownload error on 31066")
	}
	if locOut != "" || locErr != "" {
		t.Fatalf("failed RunLocateDownload must return error cleanly with empty stdout/stderr (cliapp prints once), got stdout=%q stderr=%q", locOut, locErr)
	}

	// 4. share list password fetch error must go to stderr, NOT stdout (#19).
	shareOut, shareErr, err := captureOutputs(t, func() error {
		return RunShareList(1)
	})
	if err != nil {
		t.Fatalf("RunShareList failed: %v", err)
	}
	if strings.Contains(shareOut, "获取分享") {
		t.Fatalf("share password fetch warning polluted stdout:\n%s", shareOut)
	}
	if !strings.Contains(shareErr, "991") {
		t.Fatalf("expected share password fetch warning on stderr, got:\n%s", shareErr)
	}
	_ = fmt.Sprint()
}

func TestRunShareTransferEndToEndPublicShareSecureCookieAndCollectDownload(t *testing.T) {
	origDelay := shareTransferDownloadDelay
	shareTransferDownloadDelay = 0
	defer func() { shareTransferDownloadDelay = origDelay }()

	var (
		verifyCalls     int
		sawSecureCookie bool
		mkdirPaths      []string
	)

	setupMockCommandEnv(t, func(req *http.Request) (*http.Response, error) {
		p := req.URL.Path
		q := req.URL.Query()
		for _, ck := range req.Cookies() {
			if ck.Name == "BDCLND" && (ck.Value == "pub_secure_sk" || ck.Value == "pwd_verified_sk") {
				sawSecureCookie = true
			}
		}
		switch {
		case strings.HasPrefix(p, "/s/"):
			hdr := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
			if strings.HasSuffix(p, "1PublicShareLink001") {
				hdr.Add("Set-Cookie", "BDCLND=pub_secure_sk; Domain=.baidu.com; Path=/; Secure")
			}
			html := `<html><script>locals.mset({"loginstate":1,"bdstoken":"bds_1","uk":"100","share_uk":"200","shareid":"300"});</script></html>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(html)),
				Request:    req,
			}, nil
		case p == "/share/verify":
			verifyCalls++
			hdr := http.Header{
				"Content-Type": []string{"application/json; charset=utf-8"},
				"Set-Cookie":   []string{"BDCLND=pwd_verified_sk; Domain=.baidu.com; Path=/; Secure"},
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(`{"errno":0,"randsk":"pwd_verified_sk"}`)),
				Request:    req,
			}, nil
		case p == "/share/list":
			return jsonHTTPResponse(`{"errno":0,"list":[{"fs_id":701,"server_filename":"bundle.zip"},{"fs_id":702,"server_filename":"notes.txt"}]}`), nil
		case p == "/share/transfer":
			return jsonHTTPResponse(`{"errno":0,"info":[{"fsid":701,"path":"/work/bundle.zip"},{"fsid":702,"path":"/work/notes.txt"}]}`), nil
		case q.Get("method") == "mkdir":
			mkdirPaths = append(mkdirPaths, q.Get("path"))
			return jsonHTTPResponse(`{"error_code":0}`), nil
		case q.Get("method") == "meta":
			var bodyBytes []byte
			if req.Body != nil {
				bodyBytes, _ = io.ReadAll(req.Body)
			}
			target := string(bodyBytes)
			if strings.Contains(target, "bundle.zip") && !strings.Contains(target, "notes.txt") {
				return jsonHTTPResponse(`{"error_code":0,"list":[{"fs_id":701,"path":"/work/bundle.zip等文件/bundle.zip","server_filename":"bundle.zip","size":0,"isdir":0}]}`), nil
			}
			return jsonHTTPResponse(`{"error_code":0,"list":[{"fs_id":702,"path":"/work/bundle.zip等文件/notes.txt","server_filename":"notes.txt","size":0,"isdir":0}]}`), nil
		default:
			return jsonHTTPResponse(`{"error_code":0,"errno":0}`), nil
		}
	})

	// 1. Password-free public share (extraCode == ""): must NOT call /share/verify, and UpdatePCSCookies must keep Secure BDCLND cookie!
	_, _, err := captureOutputs(t, func() error {
		return RunShareTransfer([]string{"pan.baidu.com/s/1PublicShareLink001"}, &baidupcs.TransferOption{})
	})
	if err != nil {
		t.Fatalf("public share RunShareTransfer failed: %v", err)
	}
	if verifyCalls != 0 {
		t.Fatalf("expected 0 /share/verify calls on password-free public share, got %d", verifyCalls)
	}
	if !sawSecureCookie {
		t.Fatal("UpdatePCSCookies dropped Secure BDCLND cookie set on https://pan.baidu.com")
	}

	// 2. Password-protected share with --collect --download: creates collection dir and downloads prefixed paths!
	sawSecureCookie = false
	_, _, err = captureOutputs(t, func() error {
		return RunShareTransfer(
			[]string{"链接: https://pan.baidu.com/s/1VYzSl7465sdrQXe8GT5RdQ?pwd=704e提取码: 704e"},
			&baidupcs.TransferOption{Collect: true, Download: true},
		)
	})
	if err != nil {
		t.Fatalf("RunShareTransfer --collect --download failed: %v", err)
	}
	if verifyCalls != 1 || !sawSecureCookie {
		t.Fatalf("expected 1 verify call and preserved Secure cookie, got verifyCalls=%d sawSecure=%v", verifyCalls, sawSecureCookie)
	}
	if len(mkdirPaths) != 1 || mkdirPaths[0] != "/work/bundle.zip等文件" {
		t.Fatalf("expected Mkdir('/work/bundle.zip等文件'), got %#v", mkdirPaths)
	}
	saveBase := pcsconfig.Config.ActiveUser().GetSavePath("")
	for _, wantName := range []string{"bundle.zip", "notes.txt"} {
		matches, _ := filepath.Glob(filepath.Join(saveBase, "*", wantName))
		if len(matches) == 0 {
			// Check direct saveBase/wantName as well (PreBase == "" for direct file download)
			if _, statErr := os.Stat(filepath.Join(saveBase, wantName)); statErr != nil {
				t.Fatalf("expected downloaded file %q under %s, stat err: %v", wantName, saveBase, statErr)
			}
		}
	}
}

func TestRunGetMetaPreservesFinalTableRows(t *testing.T) {
	setupMockCommandEnv(t, func(req *http.Request) (*http.Response, error) {
		return jsonHTTPResponse(`{"error_code":0,"list":[{"fs_id":8801,"app_id":266719,"path":"/work/docs","server_filename":"docs","isdir":1,"ctime":1700000000,"mtime":1700003600,"ifhassubdir":1}]}`), nil
	})
	ui.SetNoColor(true)
	stdout, _, err := captureOutputs(t, func() error {
		return RunGetMeta("/work/docs")
	})
	if err != nil {
		t.Fatalf("RunGetMeta failed: %v", err)
	}
	for _, want := range []string{"目录路径", "修改日期", "是否含有子目录", "true"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("RunGetMeta output missing %q:\n%s", want, stdout)
		}
	}
}
