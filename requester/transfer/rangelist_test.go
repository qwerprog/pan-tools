package transfer

import (
	"sync"
	"testing"
)

func TestRangeListGenZeroParallelAndOffsetRemainder(t *testing.T) {
	gen := NewRangeListGenDefault(1024*1024, 0, 0, 0)
	if bs := gen.LoadBlockSize(); bs <= 0 {
		t.Fatalf("expected positive blockSize when parallel=0, got %d", bs)
	}

	// total=1000, begin=300 => remaining=700; with blockSize=500 => 2 ranges (500 + 200)
	blockGen := NewRangeListGenBlockSize(1000, 300, 500)
	if got := blockGen.RangeCount(); got != 2 {
		t.Fatalf("RangeCount() = %d, want 2 (1000%%500==0 must not hide remaining 700%%500!=0)", got)
	}

	zeroBlockGen := NewRangeListGenBlockSize(1000, 0, 0)
	if got := zeroBlockGen.RangeCount(); got != 0 {
		t.Fatalf("RangeCount() with blockSize=0 = %d, want 0", got)
	}
}

func TestRangeListGenConcurrentAccessRaceFree(t *testing.T) {
	for _, gen := range []*RangeListGen{
		NewRangeListGenDefault(8*1024*1024, 0, 0, 8),
		NewRangeListGenBlockSize(8*1024*1024, 0, 256*1024),
	} {
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					_ = gen.RangeGenMode()
					_ = gen.RangeCount()
					_ = gen.LoadBlockSize()
					_ = gen.LoadBegin()
					if gen.IsDone() {
						return
					}
					_, r := gen.GenRange()
					if r == nil {
						return
					}
				}
			}()
		}
		wg.Wait()
		if !gen.IsDone() {
			t.Fatal("expected generator IsDone() == true after draining")
		}
	}
}
