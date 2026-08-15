// Package fvecs reads the .fvecs and .ivecs files the SIFT and GIST datasets
// ship in.
//
// One record per vector: a 4 byte little endian int32 holding the dimension,
// then that many 4 byte values. .fvecs holds float32, .ivecs int32. No header
// and no count, so the number of vectors has to come from the file size.
package fvecs

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// Anything wider than this is a corrupt file, not a real vector. GIST is 960.
const maxDim = 1 << 16

// Float holds every vector in one flat slice. One allocation instead of a
// million, and the scan reads them in order.
type Float struct {
	Dim  int
	Data []float32
}

func (s *Float) Len() int { return len(s.Data) / s.Dim }

// At returns a window into Data, not a copy. The capacity is clamped to the end
// of the vector so an append can't run into the next one.
func (s *Float) At(i int) []float32 {
	return s.Data[i*s.Dim : (i+1)*s.Dim : (i+1)*s.Dim]
}

// Int is the same for .ivecs. Ground truth files are one record per query,
// holding that query's nearest neighbour IDs.
type Int struct {
	Dim  int
	Data []int32
}

func (s *Int) Len() int { return len(s.Data) / s.Dim }

func (s *Int) At(i int) []int32 {
	return s.Data[i*s.Dim : (i+1)*s.Dim : (i+1)*s.Dim]
}

// TODO: SIFT1M is 512 MB and this reads all of it up front. mmap would start
// serving immediately and let the OS page in what's touched. Not urgent while
// everything fits in RAM.
func ReadFloat(path string) (*Float, error) {
	f, r, dim, count, err := open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	set := &Float{Dim: dim, Data: make([]float32, count*dim)}
	buf := make([]byte, dim*4)

	for i := 0; i < count; i++ {
		if err := record(r, buf, dim, i, path); err != nil {
			return nil, err
		}
		out := set.Data[i*dim : (i+1)*dim]
		for j := range out {
			out[j] = math.Float32frombits(binary.LittleEndian.Uint32(buf[j*4:]))
		}
	}
	return set, nil
}

func ReadInt(path string) (*Int, error) {
	f, r, dim, count, err := open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	set := &Int{Dim: dim, Data: make([]int32, count*dim)}
	buf := make([]byte, dim*4)

	for i := 0; i < count; i++ {
		if err := record(r, buf, dim, i, path); err != nil {
			return nil, err
		}
		out := set.Data[i*dim : (i+1)*dim]
		for j := range out {
			out[j] = int32(binary.LittleEndian.Uint32(buf[j*4:]))
		}
	}
	return set, nil
}

// open reads the first record's dimension, works out how many records the file
// must hold, and leaves the reader just past that first header.
func open(path string) (*os.File, *bufio.Reader, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, 0, err
	}

	fail := func(err error) (*os.File, *bufio.Reader, int, int, error) {
		f.Close()
		return nil, nil, 0, 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	size := info.Size()
	if size == 0 {
		return fail(fmt.Errorf("%s: file is empty", path))
	}

	r := bufio.NewReaderSize(f, 1<<20)

	var head [4]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return fail(fmt.Errorf("%s: reading first dimension: %w", path, err))
	}
	dim := int(binary.LittleEndian.Uint32(head[:]))
	if dim <= 0 || dim > maxDim {
		return fail(fmt.Errorf("%s: dimension %d is out of range, probably not a vecs file", path, dim))
	}

	// Fixed size records, so anything that isn't an exact multiple is truncated.
	recSize := int64(4 + dim*4)
	if size%recSize != 0 {
		return fail(fmt.Errorf("%s: size %d is not a multiple of record size %d, file looks truncated", path, size, recSize))
	}

	return f, r, dim, int(size / recSize), nil
}

func record(r *bufio.Reader, buf []byte, dim, i int, path string) error {
	// open already consumed the first header.
	if i > 0 {
		var head [4]byte
		if _, err := io.ReadFull(r, head[:]); err != nil {
			return fmt.Errorf("%s: reading dimension of vector %d: %w", path, i, err)
		}
		if d := int(binary.LittleEndian.Uint32(head[:])); d != dim {
			return fmt.Errorf("%s: vector %d has dimension %d, expected %d", path, i, d, dim)
		}
	}
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("%s: reading vector %d: %w", path, i, err)
	}
	return nil
}
