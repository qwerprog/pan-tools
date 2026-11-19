package downloader

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
)

func TestBufferPoolResizeAndConcurrency(t *testing.T) {
	setBufferPoolSize(4096)
	b1 := acquireBuffer()
	if len(b1) != 4096 {
		t.Fatalf("got len %d, want 4096", len(b1))
	}
	releaseBuffer(b1)

	// 扩容后旧小切片不应被截断越界，必须返回 >= 新大小的新切片
	setBufferPoolSize(32768)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := acquireBuffer()
			if len(buf) != 32768 {
				t.Errorf("got len %d, want 32768", len(buf))
			}
			buf[0] = 1
			buf[len(buf)-1] = 2
			releaseBuffer(buf)
		}()
	}
	wg.Wait()
}

func TestWorkerStatusAtomicAndResetController(t *testing.T) {
	ws := NewWorkerStatus()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ws.SetStatusCode(StatusCode(j % 10))
				_ = ws.StatusCode()
				_ = ws.StatusText()
			}
		}(i)
	}
	wg.Wait()

	rc := NewResetController(2)
	if !rc.CanReset() {
		t.Fatal("expected initial CanReset=true")
	}
	rc.AddResetNum()
	rc.AddResetNum()
	if rc.CanReset() {
		t.Fatal("expected CanReset=false after reaching maxResetNum=2")
	}
}

func TestDownloaderFastCompletionWithoutPollingLag(t *testing.T) {
	payload := bytes.Repeat([]byte("pan-tools-Downloader-Test-"), 512)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "sample.bin", time.Now(), bytes.NewReader(payload))
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "sample.bin")
	writer, file, err := NewDownloaderWriterByFilename(outPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("open writer failed: %v", err)
	}
	defer file.Close()

	cfg := NewConfig()
	cfg.MaxParallel = 2
	cfg.CacheSize = 4096
	cfg.InstanceStatePath = outPath + ".downloading"

	der := NewDownloader(srv.URL+"/sample.bin", writer, cfg)
	der.OnDownloadStatusEvent(func(status transfer.DownloadStatuser, workersCallback func(RangeWorkerFunc)) {
		_ = status.Downloaded()
		workersCallback(func(_ int, worker *Worker) bool {
			_ = worker.GetSpeedsPerSecond()
			return true
		})
	})
	start := time.Now()
	if err := der.Execute(); err != nil {
		t.Fatalf("Downloader.Execute failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed >= 800*time.Millisecond {
		t.Fatalf("expected event-driven Monitor completion <800ms, took %v", elapsed)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output failed: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("downloaded content mismatch: got %d bytes, want %d bytes (%s)", len(got), len(payload), fmt.Sprintf("%x", got[:16]))
	}
}

func TestMonitorConcurrentReloadRangeWorkerAndInstanceStatePermissions(t *testing.T) {
	// 1. InstanceState file permissions (0600) and closed-file safety without panic (#26, #27).
	statePath := filepath.Join(t.TempDir(), "bp.state")
	der := NewDownloader("http://127.0.0.1/unused", nil, &Config{
		InstanceStatePath: statePath,
	})
	if err := der.initInstanceState(InstanceStateStorageFormatJSON); err != nil {
		t.Fatalf("initInstanceState failed: %v", err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(statePath)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0600 {
			t.Fatalf("instance state perm = %04o, want 0600", perm)
		}
	}
	_ = der.instanceState.Close()
	// Calling Get() after Close() previously panicked on Stat() (#27).
	if got := der.instanceState.Get(); got != nil {
		t.Fatalf("expected nil from closed InstanceState.Get(), got %+v", got)
	}

	// 2. Concurrent Monitor Execute + RangeWorker + Worker Reset/Cancel race test (#2).
	payload := bytes.Repeat([]byte("concurrent-monitor-worker-slice-"), 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Millisecond)
		http.ServeContent(w, r, "blob.bin", time.Now(), bytes.NewReader(payload))
	}))
	defer srv.Close()

	mt := NewMonitor()
	mt.SetReloadWorker(true)
	st := transfer.NewDownloadStatus()
	st.SetTotalSize(int64(len(payload)))
	mt.SetStatus(st)

	var bufWrites sync.Mutex
	out := make([]byte, len(payload))
	sliceWriter := writerAtFunc(func(p []byte, off int64) (int, error) {
		copy(out[off:], p)
		return len(p), nil
	})

	half := int64(len(payload) / 2)
	w0 := NewWorker(0, srv.URL+"/blob.bin", sliceWriter)
	w0.SetAcceptRange("bytes")
	w0.SetRange(&transfer.Range{Begin: 0, End: half})
	w0.SetWriteMutex(&bufWrites)

	w1 := NewWorker(1, srv.URL+"/blob.bin", sliceWriter)
	w1.SetAcceptRange("bytes")
	w1.SetRange(&transfer.Range{Begin: half, End: int64(len(payload))})
	w1.SetWriteMutex(&bufWrites)

	mt.Append(w0)
	mt.Append(w1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		mt.Execute(ctx)
	}()

	for {
		select {
		case <-done:
			if err := mt.Err(); err != nil {
				t.Fatalf("Monitor.Err() = %v", err)
			}
			if !bytes.Equal(out, payload) {
				t.Fatal("Monitor parallel download payload mismatch")
			}
			return
		default:
			mt.RangeWorker(func(_ int, worker *Worker) bool {
				_ = worker.GetStatus().StatusCode()
				_ = worker.GetRange().ShowDetails()
				return true
			})
			time.Sleep(2 * time.Millisecond)
		}
	}
}

type writerAtFunc func(p []byte, off int64) (int, error)

func (f writerAtFunc) WriteAt(p []byte, off int64) (int, error) {
	return f(p, off)
}

func TestDownloaderCancelAndEmptyMonitorCloseCompletedWithoutRace(t *testing.T) {
	emptyMt := NewMonitor()
	emptyMt.Execute(context.Background())
	select {
	case <-emptyMt.CompletedChan():
	default:
		t.Fatal("expected empty Monitor.Execute to close CompletedChan")
	}

	blockCh := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", "65536")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(bytes.Repeat([]byte("a"), 1024))
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		<-blockCh
	}))
	defer func() {
		close(blockCh)
		srv.Close()
	}()

	cfg := NewConfig()
	cfg.IsTest = true
	cfg.MaxParallel = 2
	der := NewDownloader(srv.URL+"/slow.bin", nil, cfg)
	der.SetFirstInfo(&DownloadFirstInfo{ContentLength: 65536, AcceptRanges: "bytes"})
	der.OnDownloadStatusEvent(func(_ transfer.DownloadStatuser, cb func(RangeWorkerFunc)) {
		cb(func(_ int, w *Worker) bool {
			_ = w.GetSpeedsPerSecond()
			return true
		})
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- der.Execute()
	}()
	time.Sleep(40 * time.Millisecond)
	der.Cancel()

	if err := <-errCh; err == nil {
		t.Fatal("expected canceled Downloader.Execute() to return non-nil error")
	}
}
