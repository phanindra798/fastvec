package index

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sync"
)

// On-disk format. Little endian throughout.
//
//	magic    8 bytes "FASTVEC\x01"
//	header   dim, count, m, mMax, mMax0, efConstruct, efSearch,
//	         maxLevel, entry, flags   as uint32
//	         seed                     as int64
//	per node uint8 how many levels it occupies
//	per node per level: uint32 neighbour count, then that many uint32 IDs
//	vectors  count*dim float32
//
// Vectors are stored alongside the graph so a loaded index is usable on its
// own. That makes the file about the size of the dataset, which for SIFT1M is
// 512 MB of the 550 MB total.
const (
	storeMagic   = "FASTVEC\x01"
	storeVersion = 1

	flagSingleLayer = 1 << 0
	flagNearestM    = 1 << 1

	// Ceiling on count*dim so a corrupt header cannot trigger a huge
	// allocation. Two billion float32 is 8 GB, well past anything intended.
	maxElements = 2 << 30
)

func (h *HNSW) Save(w io.Writer) error {
	bw := bufio.NewWriterSize(w, 1<<20)

	if _, err := bw.WriteString(storeMagic); err != nil {
		return err
	}

	var flags uint32
	if h.singleLayer {
		flags |= flagSingleLayer
	}
	if h.nearestM {
		flags |= flagNearestM
	}

	head := []uint32{
		storeVersion,
		uint32(h.dim),
		uint32(h.Len()),
		uint32(h.m),
		uint32(h.mMax),
		uint32(h.mMax0),
		uint32(h.efConstruct),
		uint32(h.EfSearch()),
		uint32(h.maxLevel),
		uint32(h.entry),
		flags,
	}
	for _, v := range head {
		if err := binary.Write(bw, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	if err := binary.Write(bw, binary.LittleEndian, h.seed); err != nil {
		return err
	}

	for _, per := range h.links {
		if len(per) > 255 {
			return fmt.Errorf("node has %d levels, format allows 255", len(per))
		}
		if err := bw.WriteByte(byte(len(per))); err != nil {
			return err
		}
	}

	// binary.Write per value costs an interface dispatch each time, and SIFT1M
	// has around 150 million of them. Encoding into a reused buffer instead
	// turns the write into a handful of syscalls.
	w32 := newWordWriter(bw)

	for _, per := range h.links {
		for _, nbs := range per {
			if err := w32.put(uint32(len(nbs))); err != nil {
				return err
			}
			for _, id := range nbs {
				if err := w32.put(uint32(id)); err != nil {
					return err
				}
			}
		}
	}

	for _, x := range h.data {
		if err := w32.put(math.Float32bits(x)); err != nil {
			return err
		}
	}

	if err := w32.flush(); err != nil {
		return err
	}
	return bw.Flush()
}

func LoadHNSW(r io.Reader) (*HNSW, error) {
	br := bufio.NewReaderSize(r, 1<<20)

	magic := make([]byte, len(storeMagic))
	if _, err := io.ReadFull(br, magic); err != nil {
		return nil, fmt.Errorf("reading magic: %w", err)
	}
	if string(magic) != storeMagic {
		return nil, fmt.Errorf("not a fastvec index file")
	}

	head := make([]uint32, 11)
	for i := range head {
		if err := binary.Read(br, binary.LittleEndian, &head[i]); err != nil {
			return nil, fmt.Errorf("reading header: %w", err)
		}
	}
	if head[0] != storeVersion {
		return nil, fmt.Errorf("index is format version %d, this build reads %d", head[0], storeVersion)
	}

	var seed int64
	if err := binary.Read(br, binary.LittleEndian, &seed); err != nil {
		return nil, err
	}

	dim, count := int(head[1]), int(head[2])
	if dim < 1 || count < 1 {
		return nil, fmt.Errorf("header says %d vectors of dimension %d", count, dim)
	}
	// A corrupt header could otherwise ask for a terabyte before anything else
	// gets a chance to notice the file is wrong.
	if int64(count)*int64(dim) > maxElements {
		return nil, fmt.Errorf("header says %d x %d, which is past anything this reads", count, dim)
	}
	if head[3] < 2 {
		return nil, fmt.Errorf("header says M is %d, must be at least 2", head[3])
	}

	h := &HNSW{
		dim:         dim,
		data:        make([]float32, count*dim),
		links:       make([][][]int32, count),
		m:           int(head[3]),
		mMax:        int(head[4]),
		mMax0:       int(head[5]),
		ml:          1 / math.Log(float64(head[3])),
		efConstruct: int(head[6]),
		maxLevel:    int(head[8]),
		entry:       int32(head[9]),
		singleLayer: head[10]&flagSingleLayer != 0,
		nearestM:    head[10]&flagNearestM != 0,
		seed:        seed,
		rng:         rand.New(rand.NewSource(seed)),
	}
	h.efSearch.Store(int64(head[7]))
	h.pool = sync.Pool{New: func() any { return newWorkspace(count) }}

	levels := make([]byte, count)
	if _, err := io.ReadFull(br, levels); err != nil {
		return nil, fmt.Errorf("reading level counts: %w", err)
	}

	r32 := newWordReader(br)

	for i, n := range levels {
		if n == 0 {
			return nil, fmt.Errorf("node %d occupies no levels", i)
		}
		h.links[i] = make([][]int32, n)

		for lc := range h.links[i] {
			size, err := r32.next()
			if err != nil {
				return nil, fmt.Errorf("node %d level %d: %w", i, lc, err)
			}
			if int(size) > count {
				return nil, fmt.Errorf("node %d level %d claims %d neighbours, index holds %d nodes", i, lc, size, count)
			}

			nbs := make([]int32, size)
			for j := range nbs {
				id, err := r32.next()
				if err != nil {
					return nil, fmt.Errorf("node %d level %d: %w", i, lc, err)
				}
				if int(id) >= count {
					return nil, fmt.Errorf("node %d has neighbour %d, index holds %d nodes", i, id, count)
				}
				nbs[j] = int32(id)
			}
			h.links[i][lc] = nbs
		}
	}

	for i := range h.data {
		bits, err := r32.next()
		if err != nil {
			return nil, fmt.Errorf("reading vector data at %d: %w", i, err)
		}
		h.data[i] = math.Float32frombits(bits)
	}

	if int(h.entry) >= count {
		return nil, fmt.Errorf("entry point is node %d, index holds %d nodes", h.entry, count)
	}
	if h.maxLevel != len(h.links[h.entry])-1 {
		return nil, fmt.Errorf("header says maxLevel %d, entry node occupies %d levels",
			h.maxLevel, len(h.links[h.entry]))
	}

	return h, nil
}

// wordWriter and wordReader move uint32s through a reusable buffer instead of
// one binary.Write or binary.Read per value. SIFT1M holds about 150 million of
// them, and the per-call overhead dominates everything else at that count.

const wordChunk = 1 << 16 // words, so 256 KB of buffer

type wordWriter struct {
	w   io.Writer
	buf []byte
	n   int
}

func newWordWriter(w io.Writer) *wordWriter {
	return &wordWriter{w: w, buf: make([]byte, wordChunk*4)}
}

func (ww *wordWriter) put(v uint32) error {
	binary.LittleEndian.PutUint32(ww.buf[ww.n:], v)
	ww.n += 4
	if ww.n == len(ww.buf) {
		return ww.flush()
	}
	return nil
}

func (ww *wordWriter) flush() error {
	if ww.n == 0 {
		return nil
	}
	_, err := ww.w.Write(ww.buf[:ww.n])
	ww.n = 0
	return err
}

type wordReader struct {
	r     io.Reader
	buf   []byte
	next_ int
	end   int
}

func newWordReader(r io.Reader) *wordReader {
	return &wordReader{r: r, buf: make([]byte, wordChunk*4)}
}

// next returns the following uint32, refilling from the reader when the buffer
// runs out. A short read at the end is fine as long as it is a whole number of
// words; anything else means the file is truncated mid value.
func (wr *wordReader) next() (uint32, error) {
	if wr.next_ == wr.end {
		n, err := io.ReadFull(wr.r, wr.buf)
		if n == 0 {
			if err == nil {
				err = io.EOF
			}
			return 0, err
		}
		if n%4 != 0 {
			return 0, io.ErrUnexpectedEOF
		}
		wr.next_, wr.end = 0, n
	}

	v := binary.LittleEndian.Uint32(wr.buf[wr.next_:])
	wr.next_ += 4
	return v, nil
}
