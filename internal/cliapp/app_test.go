package cliapp

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcscommand"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
	"github.com/urfave/cli"
)

func TestNewAppPanToolsRebrandAndFlags(t *testing.T) {
	app := NewApp("v4.0.2-test")
	if app.Name != "pant" {
		t.Fatalf("expected app.Name == 'pant', got %q", app.Name)
	}

	for _, name := range []string{"verbose", "no-color", "ascii", "prompt-style", "quiet"} {
		found := false
		for _, f := range app.Flags {
			if f.GetName() == name || strings.HasPrefix(f.GetName(), name+",") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing global flag %q", name)
		}
	}

	if app.Command("ui") == nil {
		t.Fatal("missing interactive 'ui' command")
	}

	for _, cmdName := range []string{"ls", "search", "who", "quota", "loglist"} {
		cmd := app.Command(cmdName)
		if cmd == nil {
			t.Fatalf("missing command %q", cmdName)
		}
		hasJSON, hasPlain := false, false
		for _, f := range cmd.Flags {
			if f.GetName() == "json" {
				hasJSON = true
			}
			if f.GetName() == "plain" {
				hasPlain = true
			}
		}
		if !hasJSON || !hasPlain {
			t.Fatalf("command %q must expose --json and --plain flags", cmdName)
		}
	}
}

func TestPanToolsHelpRendering(t *testing.T) {
	t.Setenv("PANT_ASCII", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	ui.SetASCIIOnly(false)
	ui.SetNoColor(true)
	defer ui.SetNoColor(false)

	var out bytes.Buffer
	app := NewApp("v4.0.2-test")
	app.Writer = &out

	cli.HelpPrinter(&out, cli.AppHelpTemplate, app)
	rendered := out.String()
	for _, want := range []string{"pant", ui.BrandTag, ui.Tagline, "pan-tools Contributors"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected help output to contain %q, got:\n%s", want, rendered)
		}
	}
	legacyBrand := "baidupcs" + "-go"
	if strings.Contains(strings.ToLower(rendered), legacyBrand) {
		t.Fatalf("help output must not contain legacy brand:\n%s", rendered)
	}
}

func TestPreScanGlobalFlagsStopsAtSubcommandAndDoubleDash(t *testing.T) {
	t.Setenv("PANT_ASCII", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	defer ui.RestoreOverrides(ui.UIOverrides{})

	// "pant run grep -q foo" and "pant offlinedl q 123" must NOT set ui.IsQuiet() (#6).
	ui.RestoreOverrides(ui.UIOverrides{})
	preScanGlobalFlags([]string{"pant", "run", "grep", "-q", "pattern"})
	if ui.IsQuiet() {
		t.Fatal("preScanGlobalFlags misparsed 'run grep -q' as global --quiet")
	}

	preScanGlobalFlags([]string{"pant", "offlinedl", "q", "123"})
	if ui.IsQuiet() {
		t.Fatal("preScanGlobalFlags misparsed 'offlinedl q' as global --quiet")
	}

	preScanGlobalFlags([]string{"pant", "--", "-q", "--ascii"})
	if ui.IsQuiet() || ui.ASCIIOnly() {
		t.Fatal("preScanGlobalFlags failed to stop at '--'")
	}

	// Top-level global flags before subcommand MUST take effect immediately.
	preScanGlobalFlags([]string{"pant", "-q", "--ascii", "--no-color", "help"})
	snap := ui.SnapshotOverrides()
	if !snap.Quiet || !snap.ASCII || !snap.NoColor {
		t.Fatalf("expected Quiet/ASCII/NoColor all true from top-level flags, got %+v", snap)
	}

	// Invalid bool value (--no-color=notabool) must NOT toggle NoColor.
	ui.RestoreOverrides(ui.UIOverrides{})
	preScanGlobalFlags([]string{"pant", "--no-color=notabool", "help"})
	if ui.SnapshotOverrides().NoColor {
		t.Fatal("invalid boolean flag value should not toggle NoColor")
	}

	// REPL session --verbose baseline must NOT be clobbered to false when running subcommands without --verbose.
	origVerbose := pcsverbose.IsVerbose
	defer func() { pcsverbose.IsVerbose = origVerbose }()
	pcsverbose.IsVerbose = true
	app := NewApp("v4.0.2-test")
	var observedDuringAction bool
	app.Commands = append(app.Commands, cli.Command{
		Name: "probe-verbose",
		Action: func(_ *cli.Context) error {
			observedDuringAction = pcsverbose.IsVerbose
			return nil
		},
	})
	if err := app.Run([]string{"pant", "probe-verbose"}); err != nil {
		t.Fatalf("probe-verbose failed: %v", err)
	}
	if !observedDuringAction {
		t.Fatal("subcommand execution clobbered pcsverbose.IsVerbose session baseline to false")
	}
}

func TestSubcommandInvalidFlagAndDownloadInvalidModeDoNotPolluteStdout(t *testing.T) {
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, t.TempDir())
	defer ui.RestoreOverrides(ui.UIOverrides{})

	captureRun := func(args []string) (stdout, stderr string, err error) {
		oldOut, oldErr := os.Stdout, os.Stderr
		outR, outW, _ := os.Pipe()
		errR, errW, _ := os.Pipe()
		os.Stdout, os.Stderr = outW, errW

		runErr := Run("v4.0.2-test", args)

		_ = outW.Close()
		_ = errW.Close()
		os.Stdout, os.Stderr = oldOut, oldErr

		var outB, errB bytes.Buffer
		_, _ = io.Copy(&outB, outR)
		_, _ = io.Copy(&errB, errR)
		return outB.String(), errB.String(), runErr
	}

	// 1. Invalid subcommand flag: stdout must be completely empty, error printed once to stderr (#6).
	stdout, stderr, err := captureRun([]string{"pant", "--no-color", "ls", "--definitely-invalid-flag"})
	if err == nil {
		t.Fatal("expected error for invalid subcommand flag")
	}
	if stdout != "" {
		t.Fatalf("invalid subcommand flag polluted stdout:\n%s", stdout)
	}
	if !strings.Contains(stderr, "definitely-invalid-flag") || strings.Contains(stderr, "\x1b[") {
		t.Fatalf("expected uncolored error on stderr honoring pre-scanned --no-color, got:\n%s", stderr)
	}

	// 2. Invalid download --mode: stdout must be completely empty (#20).
	stdout, stderr, err = captureRun([]string{"pant", "--no-color", "download", "--mode", "bogus_mode", "/a.zip"})
	if err == nil {
		t.Fatal("expected error for invalid download --mode")
	}
	if stdout != "" {
		t.Fatalf("invalid download --mode polluted stdout with help text:\n%s", stdout)
	}
	if !strings.Contains(stderr, "bogus_mode") {
		t.Fatalf("expected bogus_mode error on stderr, got:\n%s", stderr)
	}

	// 3. sumfile missing file: error must appear only once on stderr, not double-printed (#20).
	missingPath := filepath.Join(t.TempDir(), "nonexistent.bin")
	stdout, stderr, err = captureRun([]string{"pant", "--no-color", "sumfile", missingPath})
	if err == nil {
		t.Fatal("expected error for missing sumfile target")
	}
	if stdout != "" {
		t.Fatalf("failed sumfile polluted stdout:\n%s", stdout)
	}
	if count := strings.Count(stderr, "nonexistent.bin"); count != 1 {
		t.Fatalf("expected missing filename printed once to stderr, got %d occurrences:\n%s", count, stderr)
	}

	// 4. sumfile valid file: stdout must include all 4 checksum rows including crc32.
	validPath := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(validPath, []byte("pant checksum verification payload"), 0600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err = captureRun([]string{"pant", "--no-color", "sumfile", validPath})
	if err != nil || stderr != "" {
		t.Fatalf("expected sumfile to succeed without stderr, err=%v stderr=%q", err, stderr)
	}
	for _, field := range []string{"文件大小", "md5", "前256KB切片的md5", "crc32"} {
		if !strings.Contains(stdout, field) {
			t.Fatalf("sumfile stdout missing %q:\n%s", field, stdout)
		}
	}
}

func TestPromptStyleCLIFlagConfigSetEnvAndREPLFormatting(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)
	t.Setenv(ui.EnvPromptStyle, "")
	t.Setenv("PANT_ASCII", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	defer ui.RestoreOverrides(ui.UIOverrides{})

	captureRun := func(args []string) (stdout, stderr string, err error) {
		ui.RestoreOverrides(ui.UIOverrides{})
		oldOut, oldErr := os.Stdout, os.Stderr
		outR, outW, _ := os.Pipe()
		errR, errW, _ := os.Pipe()
		os.Stdout, os.Stderr = outW, errW

		runErr := Run("v4.0.2-test", args)

		_ = outW.Close()
		_ = errW.Close()
		os.Stdout, os.Stderr = oldOut, oldErr

		var outB, errB bytes.Buffer
		_, _ = io.Copy(&outB, outR)
		_, _ = io.Copy(&errB, errR)
		return outB.String(), errB.String(), runErr
	}

	// 1. Persisting prompt_style via `pant config set -prompt_style boxed`.
	stdout, stderr, err := captureRun([]string{"pant", "--no-color", "config", "set", "-prompt_style", "boxed"})
	if err != nil || stderr != "" {
		t.Fatalf("config set -prompt_style boxed failed: err=%v stderr=%q", err, stderr)
	}
	if pcsconfig.Config.PromptStyle != "boxed" || !strings.Contains(stdout, "prompt_style") {
		t.Fatalf("expected PromptStyle='boxed' and table output, got style=%q stdout=\n%s", pcsconfig.Config.PromptStyle, stdout)
	}

	app := NewApp("v4.0.2-test")
	ui.SetNoColor(true)
	user := &pcsconfig.Baidu{
		BaiduBase: pcsconfig.BaiduBase{UID: 1001, Name: "alice"},
		Workdir:   "/工作/架构设计",
	}
	meta := sessionMetaCache{
		uid:      1001,
		total:    2048 * (1 << 30),
		used:     1110 * (1 << 30),
		hasQuota: true,
		vipLabel: "SVIP",
		isVIP:    true,
		isSVIP:   true,
		hasVIP:   true,
	}
	hdr, inp := formatPrompt(app, user, meta)
	if hdr != "╭─ pan-tools ▏ ● alice [VIP: SVIP] [dir: .../架构设计] [quota: 54.2%]" || inp != "╰─❯ " {
		t.Fatalf("unexpected boxed REPL prompt: hdr=%q inp=%q", hdr, inp)
	}

	// 2. Invalid `config set -prompt_style` must fail without polluting stdout or clobbering saved 'boxed'.
	stdout, stderr, err = captureRun([]string{"pant", "--no-color", "config", "set", "-prompt_style", "bogus_style"})
	if err == nil {
		t.Fatal("expected error for invalid config set -prompt_style")
	}
	if stdout != "" {
		t.Fatalf("invalid config set -prompt_style polluted stdout:\n%s", stdout)
	}
	if !strings.Contains(stderr, "bogus_style") || strings.Contains(stderr, "\x1b[") {
		t.Fatalf("expected uncolored bogus_style error on stderr, got:\n%s", stderr)
	}
	if pcsconfig.Config.PromptStyle != "boxed" {
		t.Fatalf("failed config set clobbered PromptStyle to %q", pcsconfig.Config.PromptStyle)
	}

	// 3. Invalid top-level `--prompt-style bogus --no-color` must still pre-scan `--no-color` and keep stdout empty.
	stdout, stderr, err = captureRun([]string{"pant", "--prompt-style", "bogus_flag", "--no-color", "env"})
	if err == nil {
		t.Fatal("expected error for invalid --prompt-style flag")
	}
	if stdout != "" {
		t.Fatalf("invalid --prompt-style polluted stdout:\n%s", stdout)
	}
	if !strings.Contains(stderr, "bogus_flag") || strings.Contains(stderr, "\x1b[") {
		t.Fatalf("expected uncolored bogus_flag error on stderr, got:\n%s", stderr)
	}

	// 4. `pant env` includes PANT_PROMPT_STYLE, and `--prompt-style=starship` overrides persisted 'boxed'.
	t.Setenv(ui.EnvPromptStyle, "boxed")
	stdout, stderr, err = captureRun([]string{"pant", "--prompt-style=starship", "--no-color", "env"})
	if err != nil || stderr != "" {
		t.Fatalf("pant --prompt-style=starship env failed: err=%v stderr=%q", err, stderr)
	}
	if !strings.Contains(stdout, `PANT_PROMPT_STYLE="boxed"`) {
		t.Fatalf("expected PANT_PROMPT_STYLE in env output, got:\n%s", stdout)
	}
	legacyEnvPrefix := "BAIDUPCS" + "_GO"
	if !strings.Contains(stdout, `PANT_VERBOSE="0"`) || !strings.Contains(stdout, `PANT_CONFIG_DIR=`) || strings.Contains(stdout, legacyEnvPrefix) {
		t.Fatalf("expected PANT_VERBOSE and PANT_CONFIG_DIR without legacy env vars in env output, got:\n%s", stdout)
	}
	if ui.GetPromptStyleOverride() != "starship" {
		t.Fatalf("expected CLI override 'starship', got %q", ui.GetPromptStyleOverride())
	}
	hdrStar, inpStar := formatPrompt(app, user, meta)
	if strings.HasPrefix(hdrStar, "╭─") || inpStar != "❯ " {
		t.Fatalf("expected CLI --prompt-style=starship to override env & config, got hdr=%q inp=%q", hdrStar, inpStar)
	}

	// 5. Hyphenated alias `config set --prompt-style starship` and top-level `--prompt_style boxed` also work.
	t.Setenv(ui.EnvPromptStyle, "")
	_, stderr, err = captureRun([]string{"pant", "--no-color", "config", "set", "--prompt-style", "starship"})
	if err != nil || stderr != "" || pcsconfig.Config.PromptStyle != "starship" {
		t.Fatalf("config set --prompt-style starship failed: err=%v stderr=%q style=%q", err, stderr, pcsconfig.Config.PromptStyle)
	}
	_, stderr, err = captureRun([]string{"pant", "--prompt_style", "boxed", "--no-color", "env"})
	if err != nil || stderr != "" || ui.GetPromptStyleOverride() != "boxed" {
		t.Fatalf("pant --prompt_style boxed env failed: err=%v stderr=%q override=%q", err, stderr, ui.GetPromptStyleOverride())
	}

	// 6. `config reset` restores PromptStyle to 'starship'.
	_, _, _ = captureRun([]string{"pant", "--no-color", "config", "set", "-prompt_style", "boxed"})
	_, stderr, err = captureRun([]string{"pant", "--no-color", "config", "reset"})
	if err != nil || stderr != "" {
		t.Fatalf("config reset failed: err=%v stderr=%q", err, stderr)
	}
	if pcsconfig.Config.PromptStyle != "starship" {
		t.Fatalf("expected config reset to restore PromptStyle='starship', got %q", pcsconfig.Config.PromptStyle)
	}

	// 7. Atomicity: `config set -max_parallel 99 -prompt_style bogus` must NOT persist max_parallel=99 on disk or in memory.
	stdout, stderr, err = captureRun([]string{"pant", "--no-color", "config", "set", "-max_parallel", "99", "-prompt_style", "bogus"})
	if err == nil || stdout != "" || !strings.Contains(stderr, "bogus") {
		t.Fatalf("expected atomic failure on invalid prompt_style: err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if pcsconfig.Config.MaxParallel == 99 {
		t.Fatalf("failed config set mutated in-memory MaxParallel to %d", pcsconfig.Config.MaxParallel)
	}
	rawJSON, readErr := os.ReadFile(filepath.Join(cfgDir, pcsconfig.ConfigName))
	if readErr != nil {
		t.Fatalf("failed reading config file: %v", readErr)
	}
	if strings.Contains(string(rawJSON), `"max_parallel": 99`) {
		t.Fatalf("failed config set leaked partial mutation to pcs_config.json:\n%s", string(rawJSON))
	}

	// 8. End-to-end REPL session:
	//    - typing `--prompt-style boxed` without a subcommand must NOT spawn a nested REPL (single `quit` exits, banner printed once);
	//    - launching REPL with `--prompt-style boxed` keeps boxed prompt across non-prompt commands (`env`),
	//      then switches immediately when `config set -prompt_style starship` is executed interactively.
	runREPLSession := func(args []string, stdinText string) (stdout, stderr string, err error) {
		ui.RestoreOverrides(ui.UIOverrides{})
		oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
		inR, inW, _ := os.Pipe()
		outR, outW, _ := os.Pipe()
		errR, errW, _ := os.Pipe()
		_, _ = io.WriteString(inW, stdinText)
		_ = inW.Close()
		os.Stdin, os.Stdout, os.Stderr = inR, outW, errW

		runErr := Run("v4.0.2-test", args)

		_ = outW.Close()
		_ = errW.Close()
		_ = inR.Close()
		os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr

		var outB, errB bytes.Buffer
		_, _ = io.Copy(&outB, outR)
		_, _ = io.Copy(&errB, errR)
		return outB.String(), errB.String(), runErr
	}

	replOut, replErr, err := runREPLSession(
		[]string{"pant", "--no-color"},
		"--prompt-style boxed\nquit\n",
	)
	if err != nil {
		t.Fatalf("REPL session failed: %v (stderr=%q)", err, replErr)
	}
	if count := strings.Count(replOut, ui.BrandTag); count != 1 {
		t.Fatalf("expected welcome banner printed exactly once (no recursive nested REPL), got %d:\n%s", count, replOut)
	}
	if !strings.Contains(replErr, "config set -prompt_style") {
		t.Fatalf("expected actionable guidance when typing '--prompt-style boxed' alone in REPL, got stderr:\n%s", replErr)
	}

	replOut, replErr, err = runREPLSession(
		[]string{"pant", "--no-color", "-q", "--prompt-style", "boxed"},
		"env\nconfig set -prompt_style starship\nenv\nquit\n",
	)
	if err != nil || replErr != "" {
		t.Fatalf("REPL live switch session failed: err=%v stderr=%q", err, replErr)
	}
	// Prompts 1 & 2 (before `env` and after `env`) are `╰─❯ `; Prompt 3 (after `config set -prompt_style starship`) is `❯ `.
	if gotBoxed := strings.Count(replOut, "╰─❯ "); gotBoxed != 2 {
		t.Fatalf("expected 2 boxed prompts before interactive switch, got %d in:\n%s", gotBoxed, replOut)
	}
	if !strings.Contains(replOut, "\n❯ ") {
		t.Fatalf("expected starship prompt '❯ ' immediately after interactive config set, got:\n%s", replOut)
	}

	// 9. Launching REPL under PANT_PROMPT_STYLE=boxed and interactively executing `config set -prompt_style starship`
	//    must also switch the live REPL session prompt immediately (not stay stuck on env var 'boxed').
	t.Setenv(ui.EnvPromptStyle, "boxed")
	replOut, replErr, err = runREPLSession(
		[]string{"pant", "--no-color", "-q"},
		"env\nconfig set -prompt_style starship\nenv\nquit\n",
	)
	if err != nil || replErr != "" {
		t.Fatalf("REPL PANT_PROMPT_STYLE live switch failed: err=%v stderr=%q", err, replErr)
	}
	if gotBoxed := strings.Count(replOut, "╰─❯ "); gotBoxed != 2 {
		t.Fatalf("expected 2 boxed prompts under PANT_PROMPT_STYLE=boxed before interactive switch, got %d in:\n%s", gotBoxed, replOut)
	}
	if !strings.Contains(replOut, "\n❯ ") {
		t.Fatalf("expected starship prompt '❯ ' immediately after interactive config set under PANT_PROMPT_STYLE=boxed, got:\n%s", replOut)
	}
}

func makeMinimalValidQRPNG(t *testing.T) []byte {
	t.Helper()
	const n = 21
	const stride = 4
	img := image.NewGray(image.Rect(0, 0, n*stride+8, n*stride+8))
	for y := 0; y < n*stride+8; y++ {
		for x := 0; x < n*stride+8; x++ {
			img.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	setMod := func(c, r int, dark bool) {
		if !dark {
			return
		}
		for dy := 0; dy < stride; dy++ {
			for dx := 0; dx < stride; dx++ {
				img.SetGray(4+c*stride+dx, 4+r*stride+dy, color.Gray{Y: 0})
			}
		}
	}
	for _, org := range [][2]int{{0, 0}, {n - 7, 0}, {0, n - 7}} {
		for dy := 0; dy < 7; dy++ {
			for dx := 0; dx < 7; dx++ {
				setMod(org[0]+dx, org[1]+dy, dx == 0 || dx == 6 || dy == 0 || dy == 6 || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4))
			}
		}
	}
	for idx := 8; idx <= n-9; idx++ {
		setMod(idx, 6, idx%2 == 0)
		setMod(6, idx, idx%2 == 0)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLoginCLIFlagsInteractiveMenuAndNonInteractiveGuard(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)
	t.Setenv("PANT_ASCII", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	defer ui.RestoreOverrides(ui.UIOverrides{})

	qrPNG := makeMinimalValidQRPNG(t)

	panSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "BAIDUID", Value: "CLI_BID:FG=1", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/disk/main":
			http.SetCookie(w, &http.Cookie{Name: "STOKEN", Value: "CLI_PAN_STOKEN", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/rest/2.0/xpan/nas":
			if strings.Contains(r.Header.Get("Cookie"), "BDUSS=trigger_external_cfg_edit_then_fail") {
				cfgFile := filepath.Join(cfgDir, pcsconfig.ConfigName)
				extCfg := pcsconfig.NewConfig(cfgFile)
				if err := extCfg.Init(); err == nil {
					extCfg.MaxParallel = 17
					_ = extCfg.Save()
					_ = extCfg.Close()
				}
				_, _ = io.WriteString(w, `{"errno":-6,"errmsg":"invalid bduss"}`)
				return
			}
			_, _ = io.WriteString(w, `{"errno":0,"baidu_name":"qr_cli_user","uk":66001122}`)
		case "/api/user/getinfo":
			_, _ = io.WriteString(w, `{"errno":-6}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer panSrv.Close()

	var passportSrv *httptest.Server
	passportSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/api/getqrcode":
			_, _ = io.WriteString(w, `{"errno":0,"sign":"CLI_SIGN","imgurl":"/v2/api/qrcode?sign=CLI_SIGN"}`)
		case "/v2/api/qrcode":
			_, _ = w.Write(qrPNG)
		case "/channel/unicast":
			_, _ = io.WriteString(w, `{"errno":0,"channel_v":"{\"status\":0,\"v\":\"CLI_TICKET\"}"}`)
		case "/v3/login/main/qrbdusslogin":
			http.SetCookie(w, &http.Cookie{Name: "BDUSS", Value: "CLI_QR_BDUSS", Path: "/"})
			_, _ = io.WriteString(w, `bd__cbs__qrlogin({"code":"110000","data":{"stokenList":"['netdisk#CLI_PAN_STOKEN']"}})`)
		case "/v3/login/api/auth":
			http.Redirect(w, r, panSrv.URL+"/disk/main", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer passportSrv.Close()

	restoreQR := pcscommand.OverrideQRLoginEndpointsForTest(passportSrv.URL, panSrv.URL, 5*time.Millisecond, 3*time.Second)
	defer restoreQR()
	restoreProbes := pcsconfig.OverrideNetdiskBaseURLForTest(panSrv.URL)
	defer restoreProbes()

	runCLI := func(args []string, stdinText string) (stdout, stderr string, err error) {
		ui.RestoreOverrides(ui.UIOverrides{})
		oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
		inR, inW, _ := os.Pipe()
		outR, outW, _ := os.Pipe()
		errR, errW, _ := os.Pipe()
		if stdinText != "" {
			_, _ = io.WriteString(inW, stdinText)
		}
		_ = inW.Close()
		os.Stdin, os.Stdout, os.Stderr = inR, outW, errW

		runErr := Run("v4.0.2-test", args)

		_ = outW.Close()
		_ = errW.Close()
		_ = inR.Close()
		os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr

		var outB, errB bytes.Buffer
		_, _ = io.Copy(&outB, outR)
		_, _ = io.Copy(&errB, errR)
		return outB.String(), errB.String(), runErr
	}

	// 1. Bare `pant login` in non-interactive pipe mode MUST fail with actionable guidance (-qr / -cookies / -bduss) and keep stdout empty.
	stdout, stderr, err := runCLI([]string{"pant", "--no-color", "login"}, "")
	if err == nil {
		t.Fatal("expected bare pant login to fail in non-interactive mode")
	}
	if stdout != "" {
		t.Fatalf("non-interactive login error polluted stdout:\n%s", stdout)
	}
	for _, flagHint := range []string{"-qr", "-cookies", "-bduss"} {
		if !strings.Contains(stderr, flagHint) {
			t.Fatalf("expected non-interactive error to mention %q, got:\n%s", flagHint, stderr)
		}
	}

	// 2. Conflicting flags and orphan -stoken MUST fail cleanly without polluting stdout.
	for _, badArgs := range [][]string{
		{"pant", "--no-color", "login", "-qr", "-cookies", "BDUSS=abc"},
		{"pant", "--no-color", "login", "-qr", "-bduss", "abc"},
		{"pant", "--no-color", "login", "-stoken", "orphan_stoken"},
	} {
		outStr, errStr, badErr := runCLI(badArgs, "")
		if badErr == nil || outStr != "" || errStr == "" {
			t.Fatalf("args %v: expected error on stderr only, got err=%v stdout=%q stderr=%q", badArgs, badErr, outStr, errStr)
		}
	}

	// 3. Direct `-qr`, `--qr`, and `--qrcode` MUST succeed even when stdin is NOT a TTY!
	for _, flagForm := range []string{"-qr", "--qr", "--qrcode"} {
		stdout, stderr, err = runCLI([]string{"pant", "--no-color", "login", flagForm}, "")
		if err != nil {
			t.Fatalf("pant login %s failed: %v (stderr=%s)", flagForm, err, stderr)
		}
		if !strings.Contains(stdout, "百度帐号登录成功: qr_cli_user") {
			t.Fatalf("expected login success on stdout for %s, got:\n%s", flagForm, stdout)
		}
		if !strings.Contains(stderr, "█") {
			t.Fatalf("expected terminal QR code on stderr for %s, got:\n%s", flagForm, stderr)
		}
		active := pcsconfig.Config.ActiveUser()
		if active.BDUSS != "CLI_QR_BDUSS" || active.STOKEN != "CLI_PAN_STOKEN" {
			t.Fatalf("unexpected saved user after %s: %+v", flagForm, active)
		}
	}

	// 4. Interactive TTY mode on bare `pant login`:
	origTTYCheck := isInteractiveLoginTTY
	isInteractiveLoginTTY = func() bool { return true }
	defer func() { isInteractiveLoginTTY = origTTYCheck }()

	// 4a. Default Enter ("\n") selects option 0 ("扫码登录（推荐，支持百度网盘/百度 App）") and logs in via QR.
	stdout, stderr, err = runCLI([]string{"pant", "--no-color", "login"}, "\n")
	if err != nil {
		t.Fatalf("interactive default QR selection failed: %v (stderr=%s)", err, stderr)
	}
	if !strings.Contains(stderr, "扫码登录（推荐，支持百度网盘/百度 App）") || !strings.Contains(stdout, "qr_cli_user") {
		t.Fatalf("unexpected interactive QR output:\nstdout=%s\nstderr=%s", stdout, stderr)
	}

	// 4b. Option 2 ("粘贴 Cookies 登录"): "2\nCookie: BDUSS=paste_bduss; STOKEN=paste_stoken\n"
	_, stderr, err = runCLI([]string{"pant", "--no-color", "login"}, "2\nCookie: BDUSS=paste_bduss; STOKEN=paste_stoken; BAIDUID=paste_bid\n")
	if err != nil {
		t.Fatalf("interactive Cookie paste login failed: %v (stderr=%s)", err, stderr)
	}
	if pcsconfig.Config.ActiveUser().BDUSS != "paste_bduss" || pcsconfig.Config.ActiveUser().STOKEN != "paste_stoken" {
		t.Fatalf("unexpected active user after Cookie paste: %+v", pcsconfig.Config.ActiveUser())
	}

	// 4c. Option 3 ("输入 BDUSS + STOKEN 登录"): "\x1b[B\x1b[B\nmanual_bduss\nmanual_stoken\n"
	_, stderr, err = runCLI([]string{"pant", "--no-color", "login"}, "\x1b[B\x1b[B\nmanual_bduss\nmanual_stoken\n")
	if err != nil {
		t.Fatalf("interactive BDUSS+STOKEN login failed: %v (stderr=%s)", err, stderr)
	}
	if pcsconfig.Config.ActiveUser().BDUSS != "manual_bduss" || pcsconfig.Config.ActiveUser().STOKEN != "manual_stoken" {
		t.Fatalf("unexpected active user after manual BDUSS+STOKEN: %+v", pcsconfig.Config.ActiveUser())
	}

	// 4d. Ctrl+C ("\x03") in menu cancels cleanly without clobbering previously logged-in user!
	_, _, err = runCLI([]string{"pant", "--no-color", "login"}, "\x03")
	if !errors.Is(err, pcscommand.ErrLoginCanceled) {
		t.Fatalf("expected ErrLoginCanceled on Ctrl+C in menu, got %v", err)
	}
	if pcsconfig.Config.ActiveUser().BDUSS != "manual_bduss" {
		t.Fatalf("canceled login clobbered active user: %+v", pcsconfig.Config.ActiveUser())
	}

	// 4e. Failed/canceled login MUST NOT overwrite concurrent pcs_config.json edits made AFTER reloadFn!
	if _, _, failErr := runCLI([]string{"pant", "--no-color", "login", "-bduss=trigger_external_cfg_edit_then_fail"}, ""); failErr == nil {
		t.Fatal("expected failed login when BDUSS is rejected")
	}
	diskBytes, readErr := os.ReadFile(filepath.Join(cfgDir, pcsconfig.ConfigName))
	if readErr != nil || !strings.Contains(string(diskBytes), `"max_parallel": 17`) {
		t.Fatalf("failed login overwrote concurrent pcs_config.json edit: err=%v body=%s", readErr, string(diskBytes))
	}

	// 5. REPL session: interactive `login` canceled on EOF/abort MUST NOT kill REPL, allowing immediate `login -qr` + `who` re-entry!
	replOut, replErr, err := runCLI(
		[]string{"pant", "--no-color", "-q"},
		"login\nlogin -qr\nwho\nquit\n",
	)
	if err != nil {
		t.Fatalf("REPL login cancel + re-entry failed: err=%v stderr=%s", err, replErr)
	}
	if !strings.Contains(replErr, "已取消登录") {
		t.Fatalf("expected canceled login notice on REPL stderr, got:\n%s", replErr)
	}
	if !strings.Contains(replOut, "qr_cli_user") {
		t.Fatalf("expected REPL to survive canceled login and complete subsequent 'login -qr' + 'who', got stdout:\n%s", replOut)
	}

	// 6. When PANT_CONFIG_DIR is unset and a legacy config folder exists in $HOME, `pant login -qr`
	//    must save qrcode.png under ~/.config/pan-tools/qrcode.png and never reference the legacy folder.
	fakeHome := t.TempDir()
	legacyName := "BaiduPCS" + "-Go"
	legacyCfgDir := filepath.Join(fakeHome, ".config", legacyName)
	if err := os.MkdirAll(legacyCfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyCfgDir, pcsconfig.ConfigName), []byte(`{"appid":266719}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", fakeHome)
	t.Setenv("APPDATA", fakeHome)
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, "")
	_, stderr, err = runCLI([]string{"pant", "--no-color", "login", "-qr"}, "")
	if err != nil {
		t.Fatalf("pant login -qr with default HOME config dir failed: %v (stderr=%s)", err, stderr)
	}
	wantQRPath := filepath.Join(fakeHome, ".config", "pan-tools", "qrcode.png")
	if !strings.Contains(stderr, wantQRPath) || strings.Contains(stderr, legacyName) {
		t.Fatalf("expected QR path %q without legacy brand in stderr, got:\n%s", wantQRPath, stderr)
	}

	// 7. `pant tool enc` and `pant tool dec` default key is "pan-tools" and round-trips cleanly.
	plainFile := filepath.Join(t.TempDir(), "secret.txt")
	const secretPayload = "pan-tools tool enc/dec default key round-trip payload\n"
	if err := os.WriteFile(plainFile, []byte(secretPayload), 0600); err != nil {
		t.Fatal(err)
	}
	_, stderr, err = runCLI([]string{"pant", "--no-color", "tool", "enc", plainFile}, "")
	if err != nil || stderr != "" {
		t.Fatalf("tool enc failed: err=%v stderr=%s", err, stderr)
	}
	_, stderr, err = runCLI([]string{"pant", "--no-color", "tool", "dec", plainFile + ".encrypt"}, "")
	if err != nil || stderr != "" {
		t.Fatalf("tool dec failed: err=%v stderr=%s", err, stderr)
	}
	restored, err := os.ReadFile(plainFile)
	if err != nil || string(restored) != secretPayload {
		t.Fatalf("tool enc/dec round-trip mismatch: err=%v got=%q", err, string(restored))
	}
}
