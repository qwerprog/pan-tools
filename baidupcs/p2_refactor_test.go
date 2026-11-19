package baidupcs

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/expires"
)

func TestMatchPathByShellPatternRejectsRelativePathWithoutPanic(t *testing.T) {
	pcs := NewPCS(266719, "")
	paths, err := pcs.MatchPathByShellPattern("relative/*.txt")
	if err == nil {
		t.Fatalf("expected error on non-absolute pattern, got paths=%v", paths)
	}
	if !errors.Is(err, ErrMatchPathByShellPatternNotAbsPath) {
		t.Fatalf("expected ErrMatchPathByShellPatternNotAbsPath, got %v", err)
	}
}

func TestShareTransferTypedHelpers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/verify", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errno":0,"randsk":"mock_rand_sk"}`)
	})
	mux.HandleFunc("/verify-bad", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errno":-9}`)
	})
	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errno":0,"list":[{"fs_id":1001,"server_filename":"demo.zip"},{"fs_id":1002,"server_filename":"readme.md"}]}`)
	})
	mux.HandleFunc("/transfer", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errno":0,"info":[{"fsid":1001,"path":"/apps/demo.zip"},{"fsid":1002,"path":"/apps/readme.md"}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	pcs := NewPCS(266719, "")

	randsk, err := pcs.PostShareVerify(srv.URL+"/verify", srv.URL, map[string]string{"pwd": "abcd"})
	if err != nil || randsk != "mock_rand_sk" {
		t.Fatalf("PostShareVerify got (%q, %v), want (\"mock_rand_sk\", nil)", randsk, err)
	}

	if _, err := pcs.PostShareVerify(srv.URL+"/verify-bad", srv.URL, map[string]string{"pwd": "0000"}); err == nil || err.Error() != "提取码错误" {
		t.Fatalf("expected wrong-pwd error, got %v", err)
	}

	meta, err := pcs.ExtractShareInfo(srv.URL+"/list", "9988", "7766", "bds_tok")
	if err != nil {
		t.Fatalf("ExtractShareInfo failed: %v", err)
	}
	if meta.ItemNum != 2 || meta.FsIDListJSON != "[1001,1002]" || meta.Filename != "demo.zip" {
		t.Fatalf("unexpected ShareTransferMeta: %+v", meta)
	}
	if _, parseErr := url.Parse(meta.TransferURL); parseErr != nil {
		t.Fatalf("invalid TransferURL %q: %v", meta.TransferURL, parseErr)
	}

	// Pre-populate /apps cache to verify ExecuteShareTransfer invalidates target directory cache (#16).
	pcs.lazyInit()
	listCache := pcs.cacheOpMap.LazyInitCachePoolOp(OperationFilesDirectoriesList)
	appsCacheKey := listCacheKey("/apps", DefaultOrderOptions)
	listCache.Store(appsCacheKey, expires.NewDataExpires(FileDirectoryList{}, time.Minute))

	res, err := pcs.ExecuteShareTransfer(ShareTransferRequest{
		TransferURL:  srv.URL + "/transfer",
		Referer:      srv.URL,
		FsIDListJSON: meta.FsIDListJSON,
		TargetPath:   "/apps",
	})
	if err != nil {
		t.Fatalf("ExecuteShareTransfer failed: %v", err)
	}
	if len(res.Filenames) != 2 || res.Filenames[0] != "demo.zip" || res.Filenames[1] != "readme.md" {
		t.Fatalf("unexpected ShareTransferResult: %+v", res)
	}
	if _, stillCached := listCache.Load(appsCacheKey); stillCached {
		t.Fatal("expected ExecuteShareTransfer to invalidate /apps directory listing cache")
	}
}

func TestBaiduPCSConcurrentLazyInitCopyPCSAndCloudDlUnknownOpNoPanic(t *testing.T) {
	pcs := &BaiduPCS{appID: 266719}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%4 == 0 {
				pcs.SetPanUserAgent(NetdiskUA)
			}
			c := pcs.CopyPCS()
			_ = c.GetClient()
			_ = c.getPanUAHeader()
		}(i)
	}
	wg.Wait()

	// Unknown cloudDlManipTask op must return error cleanly instead of panicking (#27).
	if err := pcs.cloudDlManipTask("invalid_op", 123); err == nil {
		t.Fatal("expected error on unknown cloudDlManipTask op, got nil")
	}
}
