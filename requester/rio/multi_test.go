package rio

import (
	"io"
	"strings"
	"testing"
)

type readerLen64Stub struct {
	*strings.Reader
}

func (r readerLen64Stub) Len() int64 {
	return int64(r.Reader.Len())
}

func TestMultiReaderLenFiltersNil(t *testing.T) {
	rd1 := strings.NewReader("pan-")
	rd2 := strings.NewReader("tools!")
	multi := MultiReaderLen(nil, rd1, nil, rd2, nil)
	if got := multi.Len(); got != 10 {
		t.Fatalf("MultiReaderLen.Len() = %d, want 10", got)
	}
	b, err := io.ReadAll(multi)
	if err != nil || string(b) != "pan-tools!" {
		t.Fatalf("ReadAll = %q, err = %v", string(b), err)
	}
}

func TestMultiReaderLen64FiltersNil(t *testing.T) {
	rd1 := readerLen64Stub{strings.NewReader("Hello ")}
	rd2 := readerLen64Stub{strings.NewReader("World")}
	multi := MultiReaderLen64(nil, rd1, nil, rd2)
	if got := multi.Len(); got != 11 {
		t.Fatalf("MultiReaderLen64.Len() = %d, want 11", got)
	}
	b, err := io.ReadAll(multi)
	if err != nil || string(b) != "Hello World" {
		t.Fatalf("ReadAll = %q, err = %v", string(b), err)
	}
}
