package pcsdownload

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
)

func TestHandleErrorRetryPolicy(t *testing.T) {
	dtu := &DownloadTaskUnit{}

	for _, nonRetryCode := range []int{31045, 31066, 31297} {
		res := &taskframework.TaskUnitRunResult{
			Err: &pcserror.PCSErrInfo{
				ErrType: pcserror.ErrTypeRemoteError,
				ErrCode: nonRetryCode,
			},
		}
		dtu.handleError(res)
		if res.NeedRetry {
			t.Fatalf("remote error code %d should not retry", nonRetryCode)
		}
	}

	retryRemote := &taskframework.TaskUnitRunResult{
		Err: &pcserror.PCSErrInfo{
			ErrType: pcserror.ErrTypeRemoteError,
			ErrCode: 31626,
		},
	}
	dtu.handleError(retryRemote)
	if !retryRemote.NeedRetry {
		t.Fatal("transient remote error 31626 should retry")
	}

	pathErrRes := &taskframework.TaskUnitRunResult{
		Err: &os.PathError{Op: "open", Path: "/denied", Err: errors.New("permission denied")},
	}
	dtu.handleError(pathErrRes)
	if pathErrRes.NeedRetry {
		t.Fatal("local os.PathError should not retry")
	}
}

func TestFileExistAndCheckFileValid(t *testing.T) {
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(emptyPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if FileExist(emptyPath) {
		t.Fatal("0-byte file must not be treated as complete existing file")
	}

	content := []byte("pan-tools checksum verification payload")
	validPath := filepath.Join(dir, "valid.bin")
	if err := os.WriteFile(validPath, content, 0600); err != nil {
		t.Fatal(err)
	}
	if !FileExist(validPath) {
		t.Fatal("non-empty file without downloading suffix should be reported as existing")
	}

	if err := os.WriteFile(validPath+DownloadSuffix, []byte("state"), 0600); err != nil {
		t.Fatal(err)
	}
	if FileExist(validPath) {
		t.Fatal("file with active .pant-downloading suffix must not be skipped as complete")
	}
	_ = os.Remove(validPath + DownloadSuffix)

	sum := md5.Sum(content)
	md5Hex := hex.EncodeToString(sum[:])

	if err := CheckFileValid(validPath, &baidupcs.FileDirectory{
		Size:          int64(len(content)),
		MD5:           md5Hex,
		BlockListJSON: baidupcs.BlockListJSON{BlockList: []string{md5Hex}},
	}); err != nil {
		t.Fatalf("CheckFileValid failed on matching MD5: %v", err)
	}

	if err := CheckFileValid(validPath, &baidupcs.FileDirectory{
		Size:          int64(len(content)),
		MD5:           "00000000000000000000000000000000",
		BlockListJSON: baidupcs.BlockListJSON{BlockList: []string{"00000000000000000000000000000000"}},
	}); !errors.Is(err, ErrDownloadChecksumFailed) {
		t.Fatalf("expected ErrDownloadChecksumFailed on mismatched MD5, got %v", err)
	}

	if err := CheckFileValid(validPath, &baidupcs.FileDirectory{
		Size:          int64(len(content)),
		MD5:           md5Hex,
		BlockListJSON: baidupcs.BlockListJSON{BlockList: []string{"part1", "part2"}},
	}); !errors.Is(err, ErrDownloadNotSupportChecksum) {
		t.Fatalf("expected ErrDownloadNotSupportChecksum on multi-block file, got %v", err)
	}
}
