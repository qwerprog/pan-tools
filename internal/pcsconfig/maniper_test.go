package pcsconfig

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/requester"
)

func TestSetupUserByBDUSSInvalidCookiesNoPanic(t *testing.T) {
	cfg := NewConfig(filepath.Join(t.TempDir(), "pcs_config.json"))
	if _, err := cfg.SetupUserByBDUSS("", "", "", "STOKEN=abc; BAIDUID=123"); err == nil {
		t.Fatal("expected error when cookies string has no BDUSS, got nil")
	}
}

func TestNewDefaultConfigDefersPathResolutionAndManipUser(t *testing.T) {
	cfg := NewDefaultConfig()
	if cfg.configFilePath != "" {
		t.Fatalf("expected NewDefaultConfig to defer path resolution before Init/Save, got %q", cfg.configFilePath)
	}

	tmpDir := t.TempDir()
	t.Setenv(EnvPanToolsConfigDir, tmpDir)
	if err := cfg.Init(); err != nil {
		t.Fatalf("cfg.Init() failed: %v", err)
	}
	defer cfg.Close()

	wantPath := filepath.Join(tmpDir, ConfigName)
	if cfg.configFilePath != wantPath {
		t.Fatalf("got configFilePath=%q, want %q", cfg.configFilePath, wantPath)
	}

	u1 := &Baidu{BaiduBase: BaiduBase{UID: 101, Name: "Alice"}}
	u2 := &Baidu{BaiduBase: BaiduBase{UID: 202, Name: "Bob"}}
	cfg.BaiduUserList = BaiduUserList{u1, u2}

	switched, err := cfg.SwitchUser(&BaiduBase{Name: "bob"})
	if err != nil || switched.UID != 202 || cfg.BaiduActiveUID != 202 {
		t.Fatalf("SwitchUser by case-insensitive name failed: got (%+v, %v), active=%d", switched, err, cfg.BaiduActiveUID)
	}

	deleted, err := cfg.DeleteUser(&BaiduBase{UID: 202})
	if err != nil || deleted.UID != 202 {
		t.Fatalf("DeleteUser failed: got (%+v, %v)", deleted, err)
	}
	if cfg.BaiduActiveUID != 101 || len(cfg.BaiduUserList) != 1 {
		t.Fatalf("expected fallback active user 101 after deleting active user, got active=%d len=%d", cfg.BaiduActiveUID, len(cfg.BaiduUserList))
	}
}

func TestConfigReloadPropagatesFixPCSAddrSboxtknProxyAndPrintTableNoCheck(t *testing.T) {
	origUA := requester.UserAgent
	origProxy := requester.ProxyAddr
	origRules := requester.ProxyHostnameRules
	origActiveUID := Config.BaiduActiveUID
	defer func() {
		requester.UserAgent = origUA
		requester.SetGlobalProxy(origProxy)
		requester.SetProxyHostnameRules(origRules)
		requester.SetLocalTCPAddrList()
		Config.BaiduActiveUID = origActiveUID
	}()

	cfgPath := filepath.Join(t.TempDir(), "pcs_config.json")
	cfg := NewConfig(cfgPath)
	if err := cfg.Init(); err != nil {
		t.Fatalf("initial Init failed: %v", err)
	}
	defer cfg.Close()

	cfg.PCSAddr = "c2.pcs.baidu.com"
	cfg.FixPCSAddr = true
	cfg.BaiduActiveUID = 888001
	cfg.BaiduUserList = BaiduUserList{
		{
			BaiduBase: BaiduBase{UID: 888001, Name: "Alice"},
			BDUSS:     "bduss_initial",
			SBOXTKN:   "sbox_tok_1",
			COOKIES:   "BDUSS=bduss_initial; STOKEN=stoken_from_cookie",
			Workdir:   "/docs",
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	cfg.setupNewUser(cfg.BaiduUserList[0])

	pcs1 := cfg.ActiveUserBaiduPCS()
	if gotHost := pcs1.URL().Host; gotHost != "c2.pcs.baidu.com" {
		t.Fatalf("expected PCSAddr c2.pcs.baidu.com, got %q", gotHost)
	}
	panURL, _ := url.Parse("http://pan.baidu.com")
	var hasStoken, hasSboxtkn bool
	for _, ck := range pcs1.GetClient().Jar.Cookies(panURL) {
		if ck.Name == "STOKEN" && ck.Value == "stoken_from_cookie" {
			hasStoken = true
		}
		if ck.Name == "SBOXTKN" && ck.Value == "sbox_tok_1" {
			hasSboxtkn = true
		}
	}
	if !hasStoken || !hasSboxtkn {
		t.Fatalf("expected both STOKEN (from COOKIES) and SBOXTKN preserved, got stoken=%v sboxtkn=%v", hasStoken, hasSboxtkn)
	}

	// Simulate external update to pcs_config.json with SAME BaiduActiveUID (#5):
	// Reload() must still refresh activeUser, pcs, UserAgent, and Proxy!
	writerCfg := NewConfig(cfgPath)
	if err := writerCfg.Init(); err != nil {
		t.Fatal(err)
	}
	writerCfg.UserAgent = "CustomReloadUA/1.0"
	writerCfg.Proxy = "http://127.0.0.1:10809"
	writerCfg.ProxyHostnames = "pan.baidu.com"
	writerCfg.PCSAddr = "d.pcs.baidu.com"
	writerCfg.BaiduUserList[0].Workdir = "/updated_workdir"
	if err := writerCfg.Save(); err != nil {
		t.Fatal(err)
	}
	_ = writerCfg.Close()

	// Close & reload original cfg from disk while activeUser.UID == BaiduActiveUID
	_ = cfg.Close()
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if cfg.ActiveUser().Workdir != "/updated_workdir" {
		t.Fatalf("Reload skipped refreshing activeUser: got Workdir=%q, want /updated_workdir", cfg.ActiveUser().Workdir)
	}
	if cfg.ActiveUserBaiduPCS().URL().Host != "d.pcs.baidu.com" {
		t.Fatalf("Reload skipped refreshing pcs host: got %q, want d.pcs.baidu.com", cfg.ActiveUserBaiduPCS().URL().Host)
	}
	if requester.UserAgent != "CustomReloadUA/1.0" || requester.ProxyAddr != "http://127.0.0.1:10809" || requester.ProxyHostnameRules != "pan.baidu.com" {
		t.Fatalf("Reload skipped refreshing global requester state: ua=%q proxy=%q rules=%q",
			requester.UserAgent, requester.ProxyAddr, requester.ProxyHostnameRules)
	}

	// Verify BaiduUserList.String() active marker & Chinese headers (#19) and PrintTable() no_check row (#25).
	Config.BaiduActiveUID = 888001
	userTable := cfg.BaiduUserList.String()
	if !strings.Contains(userTable, "当前") || !strings.Contains(userTable, "帐龄") || !strings.Contains(userTable, "工作目录") {
		t.Fatalf("expected active marker and Chinese headers in BaiduUserList.String(), got:\n%s", userTable)
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	cfg.PrintTable()
	_ = w.Close()
	os.Stdout = oldStdout
	var outBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, r)
	if !strings.Contains(outBuf.String(), "no_check") {
		t.Fatalf("expected 'no_check' row in PrintTable() output, got:\n%s", outBuf.String())
	}

	// Verify InitDefaultConfig() resets UserAgent to DefaultUserAgent (not mutated requester.UserAgent),
	// FixPCSAddr, ProxyHostnames, and MaxDownloadRate / MaxUploadRate!
	cfg.MaxDownloadRate = 1024 * 1024
	cfg.MaxUploadRate = 512 * 1024
	cfg.FixPCSAddr = true
	cfg.ProxyHostnames = "pan.baidu.com"
	cfg.InitDefaultConfig()
	if cfg.UserAgent != requester.DefaultUserAgent || cfg.FixPCSAddr || cfg.ProxyHostnames != "" || cfg.MaxDownloadRate != 0 || cfg.MaxUploadRate != 0 {
		t.Fatalf("InitDefaultConfig left stale fields: ua=%q fix=%v rules=%q dlRate=%d ulRate=%d",
			cfg.UserAgent, cfg.FixPCSAddr, cfg.ProxyHostnames, cfg.MaxDownloadRate, cfg.MaxUploadRate)
	}
}

func TestPromptStyleConfigPersistenceValidationAndReset(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "pcs_config.json")
	cfg := NewConfig(cfgPath)
	if err := cfg.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer cfg.Close()

	if cfg.PromptStyle != "starship" {
		t.Fatalf("expected default PromptStyle 'starship', got %q", cfg.PromptStyle)
	}

	if err := cfg.SetPromptStyle("BOXED"); err != nil {
		t.Fatalf("SetPromptStyle('BOXED') failed: %v", err)
	}
	if cfg.PromptStyle != "boxed" {
		t.Fatalf("expected normalized PromptStyle 'boxed', got %q", cfg.PromptStyle)
	}
	if err := cfg.SetPromptStyle("not_a_valid_style"); err == nil {
		t.Fatal("expected error for invalid prompt_style")
	}
	if cfg.PromptStyle != "boxed" {
		t.Fatalf("failed SetPromptStyle mutated PromptStyle to %q", cfg.PromptStyle)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reloaded := NewConfig(cfgPath)
	if err := reloaded.Init(); err != nil {
		t.Fatalf("reloaded Init failed: %v", err)
	}
	defer reloaded.Close()
	if reloaded.PromptStyle != "boxed" {
		t.Fatalf("expected persisted PromptStyle 'boxed' after reload, got %q", reloaded.PromptStyle)
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	reloaded.PrintTable()
	_ = w.Close()
	os.Stdout = oldStdout
	var outBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, r)
	if !strings.Contains(outBuf.String(), "prompt_style") || !strings.Contains(outBuf.String(), "boxed") {
		t.Fatalf("expected 'prompt_style' and 'boxed' in PrintTable(), got:\n%s", outBuf.String())
	}

	// Corrupt prompt_style manually edited in JSON must normalize to 'starship' on Reload().
	_ = reloaded.Close()
	if err := os.WriteFile(cfgPath, []byte(`{"appid":266719,"prompt_style":"corrupted_value"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Reload(); err != nil {
		t.Fatalf("Reload after manual edit failed: %v", err)
	}
	if reloaded.PromptStyle != "starship" {
		t.Fatalf("expected corrupt prompt_style to normalize to 'starship', got %q", reloaded.PromptStyle)
	}

	reloaded.PromptStyle = "boxed"
	reloaded.InitDefaultConfig()
	if reloaded.PromptStyle != "starship" {
		t.Fatalf("expected InitDefaultConfig to reset PromptStyle to 'starship', got %q", reloaded.PromptStyle)
	}
}

func TestCreateBaiduPCSPreservesCookiesAndOverlaysExplicitTokens(t *testing.T) {
	cfg := NewConfig(filepath.Join(t.TempDir(), "pcs_config.json"))
	cfg.InitDefaultConfig()

	// 1. Both COOKIES and explicit STOKEN/BDUSS/SBOXTKN are present (QR login case):
	//    must preserve BAIDUID/SCRC/BDUSS_BFESS from COOKIES AND overlay explicit BDUSS/STOKEN/SBOXTKN.
	u := &Baidu{
		BaiduBase: BaiduBase{UID: 9001, Name: "qr_user"},
		BDUSS:     "explicit_bduss",
		STOKEN:    "netdisk_stoken_win",
		SBOXTKN:   "explicit_sbox",
		COOKIES:   "BAIDUID=bid_cookie_1; BDUSS=old_bduss; STOKEN=passport_stoken_lose; SCRC=scrc_1; BDUSS_BFESS=bfess_1",
	}
	pcs := cfg.createBaiduPCS(u)
	panURL, _ := url.Parse("https://pan.baidu.com")
	got := make(map[string]string)
	for _, ck := range pcs.GetClient().Jar.Cookies(panURL) {
		got[ck.Name] = ck.Value
	}
	want := map[string]string{
		"BAIDUID":     "bid_cookie_1",
		"BDUSS":       "explicit_bduss",
		"STOKEN":      "netdisk_stoken_win",
		"SBOXTKN":     "explicit_sbox",
		"SCRC":        "scrc_1",
		"BDUSS_BFESS": "bfess_1",
	}
	for k, w := range want {
		if got[k] != w {
			t.Fatalf("cookie %s = %q, want %q (all=%v)", k, got[k], w, got)
		}
	}

	// 2. COOKIES without STOKEN= still preserves BAIDUID/SCRC while adding explicit BDUSS.
	u2 := &Baidu{
		BaiduBase: BaiduBase{UID: 9002, Name: "partial_cookie_user"},
		BDUSS:     "only_bduss",
		COOKIES:   "BAIDUID=bid_cookie_2; SCRC=scrc_2",
	}
	pcs2 := cfg.createBaiduPCS(u2)
	got2 := make(map[string]string)
	for _, ck := range pcs2.GetClient().Jar.Cookies(panURL) {
		got2[ck.Name] = ck.Value
	}
	if got2["BAIDUID"] != "bid_cookie_2" || got2["SCRC"] != "scrc_2" || got2["BDUSS"] != "only_bduss" {
		t.Fatalf("expected BAIDUID, SCRC, and BDUSS preserved, got %v", got2)
	}
}

func TestSetupUserByBDUSSNetdiskHTTPSFallbackAndCookieNormalization(t *testing.T) {
	mode := "uinfo_ok"
	var lastCookieHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastCookieHeader = r.Header.Get("Cookie")
		switch r.URL.Path {
		case "/rest/2.0/xpan/nas":
			switch mode {
			case "uinfo_ok":
				_, _ = io.WriteString(w, `{"errno":0,"errmsg":"succ","baidu_name":"","netdisk_name":"PanNickUser","uk":99001122}`)
			case "uinfo_fail_getinfo_ok":
				w.WriteHeader(http.StatusBadGateway)
			default:
				_, _ = io.WriteString(w, `{"errno":-6,"errmsg":"invalid bduss"}`)
			}
		case "/api/user/getinfo":
			if mode == "uinfo_fail_getinfo_ok" {
				_, _ = io.WriteString(w, `{"errno":0,"records":[{"uk":77889900,"uname":"GetInfoUser"}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"errno":-6}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	restore := OverrideNetdiskBaseURLForTest(srv.URL)
	defer restore()

	cfg := NewConfig(filepath.Join(t.TempDir(), "pcs_config.json"))
	if err := cfg.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer cfg.Close()

	// Case A: Tieba fails, xpan/nas?method=uinfo succeeds; Cookie: prefix, FOOBDUSS decoy, and quotes normalized.
	u, err := cfg.SetupUserByBDUSS("", "", "", `Cookies: FOOBDUSS=decoy_wrong; BAIDUID=bid_99; BDUSS="real_bduss_99"; STOKEN=pan_stoken_99; SBOXTKN=sbox_99`)
	if err != nil {
		t.Fatalf("SetupUserByBDUSS via xpan uinfo fallback failed: %v", err)
	}
	if u.UID != 99001122 || u.Name != "PanNickUser" {
		t.Fatalf("unexpected fallback user: UID=%d Name=%q", u.UID, u.Name)
	}
	if u.BDUSS != "real_bduss_99" || u.STOKEN != "pan_stoken_99" || u.BAIDUID != "bid_99" || u.SBOXTKN != "sbox_99" {
		t.Fatalf("unexpected parsed fields: %+v", u)
	}
	if !strings.Contains(lastCookieHeader, "BDUSS=real_bduss_99") || !strings.Contains(lastCookieHeader, "STOKEN=pan_stoken_99") {
		t.Fatalf("expected fallback HTTP probe to carry BDUSS and STOKEN cookies, got %q", lastCookieHeader)
	}

	// Case B: Both Tieba and xpan/nas fail, secondary /api/user/getinfo?need_selfinfo=1 succeeds.
	mode = "uinfo_fail_getinfo_ok"
	u2, err := cfg.SetupUserByBDUSS("bduss_second", "", "stoken_second", "")
	if err != nil {
		t.Fatalf("SetupUserByBDUSS via getinfo secondary fallback failed: %v", err)
	}
	if u2.UID != 77889900 || u2.Name != "GetInfoUser" || cfg.BaiduActiveUID != 77889900 {
		t.Fatalf("unexpected secondary fallback user: %+v active=%d", u2, cfg.BaiduActiveUID)
	}

	// Case C: Both Tieba and Netdisk reject invalid BDUSS -> must fail WITHOUT clobbering active user 77889900.
	mode = "all_fail"
	if _, err := cfg.SetupUserByBDUSS("invalid_bduss", "", "", ""); err == nil {
		t.Fatal("expected error when both Tieba and Netdisk reject BDUSS")
	}
	if cfg.BaiduActiveUID != 77889900 || cfg.ActiveUser().Name != "GetInfoUser" {
		t.Fatalf("failed SetupUserByBDUSS clobbered active user: active=%d name=%q", cfg.BaiduActiveUID, cfg.ActiveUser().Name)
	}
	if _, err := cfg.SetupUserByBDUSS("   ", "", "", ""); err == nil {
		t.Fatal("expected error for blank BDUSS")
	}
}

func TestConfigInitAppliesProxyAndLocalAddrsEvenWithStaleActiveUID(t *testing.T) {
	origUA := requester.UserAgent
	origProxy := requester.ProxyAddr
	origRules := requester.ProxyHostnameRules
	defer func() {
		requester.UserAgent = origUA
		requester.SetGlobalProxy(origProxy)
		requester.SetProxyHostnameRules(origRules)
		requester.SetLocalTCPAddrList()
	}()

	cfgPath := filepath.Join(t.TempDir(), "pcs_config.json")
	raw := `{"appid":266719,"baidu_active_uid":999999,"baidu_user_list":[],"user_agent":"PreLoginUA/1.0","proxy":"socks5://127.0.0.1:1080","proxy_hostnames":"passport.baidu.com, pan.baidu.com","local_addrs":"127.0.0.1"}`
	if err := os.WriteFile(cfgPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := NewConfig(cfgPath)
	err := cfg.Init()
	defer cfg.Close()
	if !errors.Is(err, ErrNoSuchBaiduUser) {
		t.Fatalf("expected ErrNoSuchBaiduUser for stale baidu_active_uid, got %v", err)
	}
	if requester.UserAgent != "PreLoginUA/1.0" || requester.ProxyAddr != "socks5://127.0.0.1:1080" || requester.ProxyHostnameRules != "passport.baidu.com, pan.baidu.com" {
		t.Fatalf("Init() skipped applying global network config on ErrNoSuchBaiduUser: ua=%q proxy=%q rules=%q",
			requester.UserAgent, requester.ProxyAddr, requester.ProxyHostnameRules)
	}
}

func TestGetConfigDirAlwaysUsesPanToolsAndIgnoresLegacyDir(t *testing.T) {
	homeDir := t.TempDir()
	legacyName := "BaiduPCS" + "-Go"
	legacyDir := filepath.Join(homeDir, ".config", legacyName)
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, ConfigName), []byte(`{"appid":266719}`), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("APPDATA", homeDir)
	t.Setenv(EnvConfigDir, "")
	t.Setenv("BAIDUPCS"+"_GO_CONFIG_DIR", legacyDir)

	got := GetConfigDir()
	if strings.Contains(got, legacyName) {
		t.Fatalf("GetConfigDir() must not use legacy directory, got %q", got)
	}
	if filepath.Base(got) != "pan-tools" {
		t.Fatalf("expected GetConfigDir() base name 'pan-tools', got %q", got)
	}

	overrideDir := t.TempDir()
	t.Setenv(EnvConfigDir, overrideDir)
	if gotOverride := GetConfigDir(); gotOverride != overrideDir {
		t.Fatalf("GetConfigDir() with %s=%q returned %q", EnvConfigDir, overrideDir, gotOverride)
	}
}
