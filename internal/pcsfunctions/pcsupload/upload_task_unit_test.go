package pcsupload

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/pcsconfig"
	"github.com/qjfoidnh/BaiduPCS-Go/internal/ui"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/checksum"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/taskframework"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/uploader"
)

func attachTaskInfo(unit *UploadTaskUnit) {
	executor := taskframework.NewTaskExecutor()
	executor.Append(unit, 0)
}

func TestRunReportsOversizedFileAsFailure(t *testing.T) {
	unit := &UploadTaskUnit{
		LocalFileChecksum: &checksum.LocalFileChecksum{
			LocalFileMeta: checksum.LocalFileMeta{
				Path:   "too-large.mp4",
				Length: baidupcs.MaxUploadSize + 1,
			},
		},
	}
	attachTaskInfo(unit)

	result := unit.Run()

	if result == nil || result.Succeed || result.Err == nil {
		t.Fatalf("expected explicit failure result, got %#v", result)
	}
}

func TestRunReportsUnreadableFileAsFailure(t *testing.T) {
	unit := &UploadTaskUnit{
		LocalFileChecksum: checksum.NewLocalFileChecksum(
			filepath.Join(t.TempDir(), "missing.mp4"),
			int(baidupcs.SliceMD5Size),
		),
	}
	attachTaskInfo(unit)

	result := unit.Run()

	if result == nil || result.Succeed || result.Err == nil {
		t.Fatalf("expected explicit failure result, got %#v", result)
	}
}

func TestJustGoonReturnsPreparedFailure(t *testing.T) {
	prepared := &taskframework.TaskUnitRunResult{
		ResultMessage: "目标文件大小超过剩余空间",
		Err:           errors.New("insufficient space"),
	}
	unit := &UploadTaskUnit{
		LocalFileChecksum: &checksum.LocalFileChecksum{},
		Step:              JustGoon,
		prepareResult:     prepared,
	}
	attachTaskInfo(unit)

	result := unit.runPreparedStep()

	if result != prepared {
		t.Fatalf("expected prepared result, got %#v", result)
	}
}

func TestUploadingDatabasePermissionsAndCorruptedJSONTruncation(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)

	dbPath := filepath.Join(cfgDir, UploadingFileName)
	// Write corrupted JSON with 0644 permissions first.
	if err := os.WriteFile(dbPath, []byte("{corrupted-json-payload-that-is-very-long!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	db, err := NewUploadingDatabase()
	if err != nil {
		t.Fatalf("NewUploadingDatabase on corrupted file failed: %v", err)
	}
	defer db.Close()

	fi, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 0 {
		t.Fatalf("expected corrupted DB truncated to 0 bytes, got %d bytes", fi.Size())
	}
	if runtime.GOOS != "windows" {
		if perm := fi.Mode().Perm(); perm != 0600 {
			t.Fatalf("expected uploading DB perm 0600, got %04o", perm)
		}
	}

	// Subsequent Save() must write valid JSON without trailing garbage.
	sampleFile := filepath.Join(cfgDir, "part.bin")
	if err := os.WriteFile(sampleFile, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	meta := &checksum.LocalFileMeta{Path: sampleFile, Length: 5, ModTime: -1}
	db.UpdateUploading(meta, &uploader.InstanceState{Uploadid: "uid-123"})
	if err := db.Save(); err != nil {
		t.Fatalf("db.Save() failed: %v", err)
	}
	_ = db.Close()

	reloaded, err := NewUploadingDatabase()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	defer reloaded.Close()
	if st := reloaded.Search(meta); st == nil || st.Uploadid != "uid-123" {
		t.Fatalf("expected reloaded Uploadid='uid-123', got %+v", st)
	}
}

func TestRemoteSkipAndRsyncSameSizeMarkSucceedAndCleanDatabase(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv(pcsconfig.EnvPanToolsConfigDir, cfgDir)

	sampleFile := filepath.Join(cfgDir, "exist.bin")
	if err := os.WriteFile(sampleFile, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}

	db, err := NewUploadingDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, tc := range []struct {
		name   string
		policy string
		code   int
	}{
		{"skip_existing", baidupcs.SkipPolicy, baidupcs.ErrCodeSkipExisting},
		{"skip_31061", baidupcs.SkipPolicy, 31061},
		{"rsync_same_size", baidupcs.RsyncPolicy, baidupcs.ErrCodeRsyncSameSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lfc := checksum.NewLocalFileChecksum(sampleFile, int(baidupcs.SliceMD5Size))
			lfc.Length = 7
			lfc.ModTime = -1
			db.UpdateUploading(&lfc.LocalFileMeta, &uploader.InstanceState{Uploadid: "stale-upload"})

			var dashOut bytes.Buffer
			dash := ui.NewTransferDashboard(&dashOut, "并发上传", 2, true)
			unit := &UploadTaskUnit{
				LocalFileChecksum: lfc,
				SavePath:          "/remote/exist.bin",
				Policy:            tc.policy,
				UploadingDatabase: db,
				Dashboard:         dash,
			}
			attachTaskInfo(unit)

			remoteErr := &pcserror.PCSErrInfo{
				Operation: baidupcs.OperationUploadCreateSuperFile,
				ErrType:   pcserror.ErrTypeRemoteError,
				ErrCode:   tc.code,
				ErrMsg:    "remote skip",
			}
			res := &taskframework.TaskUnitRunResult{}
			unit.classifyUploadError(remoteErr, res)

			if !res.Succeed || res.NeedRetry || res.Err != nil {
				t.Fatalf("expected Succeed=true NeedRetry=false Err=nil on %s, got %+v", tc.name, res)
			}
			if !strings.Contains(dashOut.String(), "exist.bin") {
				t.Fatalf("expected skip completion summary written via CompleteSlot, got %q", dashOut.String())
			}
			if st := db.Search(&lfc.LocalFileMeta); st != nil {
				t.Fatalf("expected stale UploadingDatabase entry deleted on skip, still got %+v", st)
			}
		})
	}
}
