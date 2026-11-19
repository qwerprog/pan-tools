package uploader_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/transfer"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/uploader"
)

func TestSplitBlock(t *testing.T) {
	const total = int64(10000)
	blocks := uploader.SplitBlock(total, 999)
	if len(blocks) != 11 {
		t.Fatalf("len(SplitBlock(10000, 999)) = %d, want 11", len(blocks))
	}

	var cursor int64
	for i, b := range blocks {
		if b.ID != i {
			t.Fatalf("block %d ID = %d, want %d", i, b.ID, i)
		}
		if b.Range.Begin != cursor {
			t.Fatalf("block %d Begin = %d, want %d", i, b.Range.Begin, cursor)
		}
		if b.Range.End <= b.Range.Begin {
			t.Fatalf("block %d invalid range [%d, %d)", i, b.Range.Begin, b.Range.End)
		}
		cursor = b.Range.End
	}
	if cursor != total {
		t.Fatalf("final block End = %d, want %d", cursor, total)
	}
}

func TestSplitUnitRead(t *testing.T) {
	const size int64 = 65536*2 + 3432
	raw := make([]byte, int(size))
	for i := range raw {
		raw[i] = byte(i % 251)
	}

	buffer := rio.NewBuffer(raw)
	unit := uploader.NewBufioSplitUnit(buffer, transfer.Range{Begin: 2, End: size}, nil, nil)

	var out bytes.Buffer
	buf := make([]byte, 1022)
	for {
		n, err := unit.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
	}

	wantLen := size - 2
	if int64(out.Len()) != wantLen || unit.Readed() != wantLen || unit.Left() != 0 {
		t.Fatalf("got out=%d readed=%d left=%d, want %d/%d/0", out.Len(), unit.Readed(), unit.Left(), wantLen, wantLen)
	}
	if !bytes.Equal(out.Bytes(), raw[2:]) {
		t.Fatal("BufioSplitUnit read payload does not match source slice [2:]")
	}
}

func TestBufioSplitUnitSeekResetsBufferAndConcurrentReaded(t *testing.T) {
	const size int64 = 192 * 1024 // > 64KB bufio buffer
	raw := make([]byte, int(size))
	for i := range raw {
		raw[i] = byte((i*31 + 7) % 251)
	}

	buffer := rio.NewBuffer(raw)
	unit := uploader.NewBufioSplitUnit(buffer, transfer.Range{Begin: 0, End: size}, nil, nil)

	// Read 1000 bytes (triggers 64KB bufio read-ahead into internal buffer), then simulate retry Seek(0, SEEK_SET).
	partial := make([]byte, 1000)
	if n, err := io.ReadFull(unit, partial); err != nil || n != 1000 {
		t.Fatalf("partial read failed: n=%d err=%v", n, err)
	}
	if !bytes.Equal(partial, raw[:1000]) {
		t.Fatal("initial partial read mismatch")
	}

	var stopReader atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stopReader.Load() {
			_ = unit.Readed()
			_ = unit.Left()
		}
	}()

	pos, err := unit.Seek(0, os.SEEK_SET)
	if err != nil || pos != 0 {
		stopReader.Store(true)
		wg.Wait()
		t.Fatalf("Seek(0, SEEK_SET) = (%d, %v), want (0, nil)", pos, err)
	}

	reRead, err := io.ReadAll(unit)
	stopReader.Store(true)
	wg.Wait()
	if err != nil {
		t.Fatalf("ReadAll after Seek(0) failed: %v", err)
	}
	if !bytes.Equal(reRead, raw) {
		t.Fatalf("payload corrupted after Seek(0, SEEK_SET): got len %d, want %d", len(reRead), len(raw))
	}

	// Verify SEEK_CUR accounts for bufio read-ahead buffer.
	if _, err := unit.Seek(0, os.SEEK_SET); err != nil {
		t.Fatal(err)
	}
	small := make([]byte, 500)
	if _, err := io.ReadFull(unit, small); err != nil {
		t.Fatal(err)
	}
	curPos, err := unit.Seek(300, os.SEEK_CUR)
	if err != nil || curPos != 800 {
		t.Fatalf("Seek(300, SEEK_CUR) after 500B read = (%d, %v), want (800, nil)", curPos, err)
	}
	afterCur := make([]byte, 256)
	if _, err := io.ReadFull(unit, afterCur); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterCur, raw[800:1056]) {
		t.Fatal("SEEK_CUR read payload mismatch")
	}

	// Invalid whence must return error without discarding buffered read-ahead bytes.
	if _, err := unit.Seek(0, 999); err == nil {
		t.Fatal("expected error for invalid whence")
	}
	nextSlice := make([]byte, 128)
	if _, err := io.ReadFull(unit, nextSlice); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(nextSlice, raw[1056:1184]) {
		t.Fatal("failed Seek discarded bufio read-ahead buffer")
	}
}

type fakeMultiUpload struct {
	mu          sync.Mutex
	attempts    map[int]int
	received    map[int][]byte
	superMerged map[int]string
}

func (f *fakeMultiUpload) Precreate() (string, error) {
	return "https://pcs.baidu.com", nil
}

func (f *fakeMultiUpload) TmpFile(_ context.Context, _ string, _ string, partseq int, _ int64, r rio.ReaderLen64) (string, error) {
	// Read 2KB first and fail once on part 0 to force Seek(0, SEEK_SET) after 64KB buffering.
	f.mu.Lock()
	attempt := f.attempts[partseq]
	f.attempts[partseq] = attempt + 1
	f.mu.Unlock()

	if partseq == 0 && attempt == 0 {
		head := make([]byte, 2048)
		_, _ = io.ReadFull(r, head)
		return "", errors.New("transient network reset")
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	f.mu.Lock()
	f.received[partseq] = data
	f.mu.Unlock()
	return fmt.Sprintf("md5-part-%d-%d", partseq, len(data)), nil
}

func (f *fakeMultiUpload) CreateSuperFile(_ string, _ string, _ string, _ int64, checksumMap map[int]string) error {
	f.mu.Lock()
	f.superMerged = make(map[int]string, len(checksumMap))
	for k, v := range checksumMap {
		f.superMerged[k] = v
	}
	f.mu.Unlock()
	return nil
}

type sliceReaderAtLen64 struct {
	*bytes.Reader
	size int64
}

func (s sliceReaderAtLen64) Len() int64 { return s.size }

func TestMultiUploaderChunkRetryNoDeadlockNoRaceAndNilCheckNoPanic(t *testing.T) {
	// 1. Nil file / nil multiUpload must report error cleanly instead of panicking (#27).
	var gotNilErr error
	var finishCalled bool
	nilUploader := uploader.NewMultiUploader(nil, nil, nil, "/dest.bin")
	nilUploader.OnError(func(err error) { gotNilErr = err })
	nilUploader.OnFinish(func() { finishCalled = true })
	nilUploader.Execute()
	if gotNilErr == nil || !finishCalled {
		t.Fatalf("expected OnError and OnFinish without panic when file/multiUpload nil, got err=%v finish=%v", gotNilErr, finishCalled)
	}

	// 2. Multi-worker upload with retry >64KB chunk, nil onUploadStatusEvent (no updateInstanceStateChan consumer),
	// and concurrent InstanceState() polling (#1, #9).
	const blockSize int64 = 128 * 1024
	const numBlocks = 8
	raw := make([]byte, int(blockSize*numBlocks))
	for i := range raw {
		raw[i] = byte((i*17 + 3) % 253)
	}
	fm := &fakeMultiUpload{
		attempts: make(map[int]int),
		received: make(map[int][]byte),
	}
	muer := uploader.NewMultiUploader(fm, sliceReaderAtLen64{Reader: bytes.NewReader(raw), size: int64(len(raw))}, &uploader.MultiUploaderConfig{
		Parallel:  4,
		BlockSize: blockSize,
	}, "/dest.bin")

	var uploadErr error
	var succeeded bool
	muer.OnError(func(err error) { uploadErr = err })
	muer.OnSuccess(func() { succeeded = true })

	done := make(chan struct{})
	go func() {
		defer close(done)
		muer.Execute()
	}()

	// Poll InstanceState concurrently while workers write checksums.
pollLoop:
	for {
		select {
		case <-done:
			break pollLoop
		default:
			_ = muer.InstanceState()
		}
	}
	if uploadErr != nil || !succeeded {
		t.Fatalf("MultiUploader failed: err=%v succeeded=%v", uploadErr, succeeded)
	}
	if len(fm.superMerged) != numBlocks {
		t.Fatalf("expected %d merged blocks, got %d", numBlocks, len(fm.superMerged))
	}
	for i := 0; i < numBlocks; i++ {
		wantSlice := raw[int64(i)*blockSize : int64(i+1)*blockSize]
		if !bytes.Equal(fm.received[i], wantSlice) {
			t.Fatalf("block %d payload corrupted after retry", i)
		}
	}
}
