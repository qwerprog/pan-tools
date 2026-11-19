package downloader

import (
	"testing"
)

func TestRandomNumber(t *testing.T) {
	for i := 0; i < 10; i++ {
		n := RandomNumber(0, 5)
		if n < 0 || n >= 5 {
			t.Fatalf("RandomNumber out of range: %d", n)
		}
	}
}

func newSlice() [][]byte {
	s := make([][]byte, 20)
	s[0] = []byte("kjashdfiuqwheirhwuq")
	s[9] = []byte("kjashdfiuqwheirhwuq")
	return s
}

func rangeSlice(f func(key int, by []byte) bool) {
	s := newSlice()
	for k := range s {
		if s[k] == nil {
			continue
		}
		if !f(k, s[k]) {
			break
		}
	}
}

func BenchmarkRange1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var a = 0
		rangeSlice(func(key int, s []byte) bool {
			a++
			return true
		})
	}
}

func BenchmarkRange2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		s := newSlice()
		a := 0
		for k := range s {
			if s[k] == nil {
				continue
			}
			a++
		}
	}
}
