package rio

import (
	"io"
)

// MultiReaderLen 合并多个ReaderLen
func MultiReaderLen(readerLens ...ReaderLen) ReaderLen {
	r := make([]io.Reader, 0, len(readerLens))
	valid := make([]ReaderLen, 0, len(readerLens))
	for k := range readerLens {
		if readerLens[k] == nil {
			continue
		}
		r = append(r, readerLens[k])
		valid = append(valid, readerLens[k])
	}
	return &multiReaderLen{
		mrls:        valid,
		multiReader: io.MultiReader(r...),
	}
}

type multiReaderLen struct {
	mrls        []ReaderLen
	multiReader io.Reader
}

func (mrl *multiReaderLen) Read(p []byte) (n int, err error) {
	return mrl.multiReader.Read(p)
}

func (mrl *multiReaderLen) Len() int {
	var i int
	for k := range mrl.mrls {
		i += mrl.mrls[k].Len()
	}
	return i
}

// MultiReaderLen64 合并多个ReaderLen64
func MultiReaderLen64(readerLen64s ...ReaderLen64) ReaderLen64 {
	r := make([]io.Reader, 0, len(readerLen64s))
	valid := make([]ReaderLen64, 0, len(readerLen64s))
	for k := range readerLen64s {
		if readerLen64s[k] == nil {
			continue
		}
		r = append(r, readerLen64s[k])
		valid = append(valid, readerLen64s[k])
	}
	return &multiReaderLen64{
		mrl64s:      valid,
		multiReader: io.MultiReader(r...),
	}
}

type multiReaderLen64 struct {
	mrl64s      []ReaderLen64
	multiReader io.Reader
}

func (mrl64 *multiReaderLen64) Read(p []byte) (n int, err error) {
	return mrl64.multiReader.Read(p)
}

func (mrl64 *multiReaderLen64) Len() int64 {
	var l int64
	for k := range mrl64.mrl64s {
		l += mrl64.mrl64s[k].Len()
	}
	return l
}
