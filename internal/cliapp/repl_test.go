package cliapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode"

	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
)

func TestNewCompleterCommandsAndSubcommands(t *testing.T) {
	app := NewApp("v4.0.2-dev")
	complete := newCompleter(app)

	top := complete("quo")
	if len(top) != 1 || top[0] != "quota " {
		t.Fatalf("complete(\"quo\") = %#v, want [\"quota \"]", top)
	}

	subs := complete("recycle ")
	foundList := false
	for _, item := range subs {
		if item == "recycle list " {
			foundList = true
			break
		}
	}
	if !foundList {
		t.Fatalf("expected 'recycle list ' in subcommand completions, got %#v", subs)
	}
}

func TestCompleteLocalPaths(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "report 2026.pdf"), []byte("pdf"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}

	got := completeLocalPaths([]string{"upload"}, tmp+"/", false, unicode.IsSpace)
	var hasFile, hasDir bool
	for _, cand := range got {
		if strings.Contains(cand, `report\ 2026.pdf `) {
			hasFile = true
		}
		if strings.HasSuffix(cand, "subdir/") {
			hasDir = true
		}
	}
	if !hasFile || !hasDir {
		t.Fatalf("unexpected local path completions: %#v", got)
	}
}

func TestShareCompletionAndREPLQuotaRefreshRules(t *testing.T) {
	var listHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listHits.Add(1)
		fmt.Fprint(w, `{"errno":0,"list":[{"fs_id":11,"path":"/demo.txt","server_filename":"demo.txt","isdir":0}]}`)
	}))
	defer srv.Close()

	srvURL, _ := url.Parse(srv.URL)
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, t.TempDir())
	_ = pcsconfig.Config.Init()
	pcsconfig.Config.EnableHTTPS = false
	pcsconfig.Config.BaiduActiveUID = 9001
	pcsconfig.Config.BaiduUserList = pcsconfig.BaiduUserList{
		{BaiduBase: pcsconfig.BaiduBase{UID: 9001, Name: "u1"}, Workdir: "/"},
	}
	_, _ = pcsconfig.Config.SwitchUser(&pcsconfig.BaiduBase{UID: 9001})
	// Note: pan.baidu.com requests won't hit srv unless transport redirects, so even if attempted it fails fast on closed port;
	// verify via custom RoundTripper that "share " and "share cancel " NEVER even invoke HTTP client!
	var remoteCalls atomic.Int32
	pcs := pcsconfig.Config.ActiveUserBaiduPCS()
	pcs.GetClient().Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		remoteCalls.Add(1)
		req.URL.Scheme = srvURL.Scheme
		req.URL.Host = srvURL.Host
		return http.DefaultTransport.RoundTrip(req)
	})

	app := NewApp("v4.0.2-dev")
	complete := newCompleter(app)

	// 1. "share " must only complete subcommands, with 0 remote list calls (#23).
	shareSubs := complete("share ")
	if remoteCalls.Load() != 0 {
		t.Fatalf("'share <Tab>' triggered %d remote list calls, want 0", remoteCalls.Load())
	}
	if len(shareSubs) == 0 {
		t.Fatal("expected subcommand completions for 'share '")
	}

	// 2. "share cancel " must NOT complete remote paths (#23).
	cancelComps := complete("share cancel ")
	if len(cancelComps) != 0 || remoteCalls.Load() != 0 {
		t.Fatalf("'share cancel <Tab>' should not complete remote paths: comps=%#v remoteCalls=%d", cancelComps, remoteCalls.Load())
	}

	// 3. "share set " MUST complete remote paths (#23).
	setComps := complete("share set ")
	if remoteCalls.Load() == 0 || len(setComps) != 1 || setComps[0] != "share set demo.txt " {
		t.Fatalf("expected 'share set <Tab>' to complete remote path 'share set demo.txt ', got %#v (calls=%d)", setComps, remoteCalls.Load())
	}

	// 4. Verify quota-mutating commands (including aliases) and offline attempted flag (#23).
	for _, args := range [][]string{
		{"u", "a.txt", "/"},
		{"rm", "/a.txt"},
		{"cp", "/a", "/b"},
		{"tf", "https://pan.baidu.com/s/1abc"},
		{"rc", "delete", "-all"},
		{"od", "add", "http://example.com"},
		{"browse"},
		{"--prompt-style", "boxed", "u", "a.txt", "/"},
		{"--prompt-style=starship", "rm", "/a.txt"},
		{"-q", "--prompt_style", "boxed", "u", "a.txt", "/"},
		{"--", "u", "a.txt", "/"},
	} {
		resolved := resolveREPLCommandName(app, args)
		if !isQuotaMutatingCommand(resolved) {
			t.Fatalf("expected %v (resolved=%q) to be recognized as quota-mutating command", args, resolved)
		}
	}

	store := &sessionMetaStore{}
	store.refresh(pcsconfig.Config.ActiveUser(), 10*time.Millisecond)
	snap := store.snapshotFor(9001)
	if !snap.attempted {
		t.Fatal("expected sessionMetaStore.refresh to mark attempted=true even on non-TTY / offline")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
