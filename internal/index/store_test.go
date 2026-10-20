package index

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"strings"
	"testing"
)

// A loaded index has to answer exactly what the built one did. Anything less
// and a saved index is a different index wearing the same name.
func TestSaveLoadRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(51))
	set := randomSet(r, 2000, 16)

	built, err := BuildHNSW(set, Params{M: 8, EfConstruct: 60, EfSearch: 50, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := built.Save(&buf); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d nodes, %d dims, %d bytes on disk", built.Len(), built.Dim(), buf.Len())

	loaded, err := LoadHNSW(&buf)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Len() != built.Len() || loaded.Dim() != built.Dim() {
		t.Fatalf("loaded %dx%d, built %dx%d", loaded.Len(), loaded.Dim(), built.Len(), built.Dim())
	}
	if loaded.MaxLevel() != built.MaxLevel() {
		t.Errorf("maxLevel %d, want %d", loaded.MaxLevel(), built.MaxLevel())
	}
	if loaded.EfSearch() != built.EfSearch() {
		t.Errorf("efSearch %d, want %d", loaded.EfSearch(), built.EfSearch())
	}

	bc, bl := built.Components()
	lc, ll := loaded.Components()
	if bc != lc || bl != ll {
		t.Errorf("loaded has %d components largest %d, built had %d and %d", lc, ll, bc, bl)
	}

	for q := 0; q < 300; q++ {
		query := make([]float32, 16)
		for i := range query {
			query[i] = r.Float32()
		}

		want, err := built.Search(query, 10)
		if err != nil {
			t.Fatal(err)
		}
		got, err := loaded.Search(query, 10)
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != len(want) {
			t.Fatalf("query %d: %d results, want %d", q, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("query %d result %d: %+v, want %+v", q, i, got[i], want[i])
			}
		}
	}
}

// Use a tiny known graph so corruptions exercise specific format invariants,
// independently of random level assignment or construction heuristics.
func TestLoadValidatesGraph(t *testing.T) {
	fixture := func() *HNSW {
		h := &HNSW{
			dim: 1, data: []float32{0, 1, 2},
			m: 2, mMax: 2, mMax0: 4, efConstruct: 4,
			entry: 0, maxLevel: 1,
			links: [][][]int32{{{1, 2}, {1}}, {{0, 2}, {0}}, {{0, 1}}},
		}
		h.SetEfSearch(4)
		return h
	}
	encode := func(t *testing.T, h *HNSW) []byte {
		t.Helper()
		var b bytes.Buffer
		if err := h.Save(&b); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	t.Run("valid graph remains searchable", func(t *testing.T) {
		h, err := LoadHNSW(bytes.NewReader(encode(t, fixture())))
		if err != nil {
			t.Fatal(err)
		}
		got, err := h.Search([]float32{2}, 3)
		if err != nil || len(got) != 3 || got[0].ID != 2 || got[0].Dist != 0 {
			t.Fatalf("unexpected search: %v, %v", got, err)
		}
		if sizes := h.LevelSizes(); len(sizes) != 2 || sizes[0] != 3 || sizes[1] != 2 {
			t.Fatalf("unexpected levels: %v", sizes)
		}
	})
	for _, tc := range []struct {
		name   string
		mutate func(*HNSW)
		want   string
	}{
		{"negative entry", func(h *HNSW) { h.entry = -1 }, "entry point"},
		{"entry past end", func(h *HNSW) { h.entry = 3 }, "entry point"},
		{"entry below top", func(h *HNSW) { h.entry = 2 }, "entry node"},
		{"zero levels", func(h *HNSW) { h.links[2] = nil }, "level count"},
		{"node above top", func(h *HNSW) { h.links[2] = make([][]int32, 3) }, "level count"},
		{"impossible top", func(h *HNSW) { h.maxLevel = 255 }, "maximum level"},
		{"single layer mismatch", func(h *HNSW) { h.singleLayer = true }, "maximum level"},
		{"negative neighbour", func(h *HNSW) { h.links[0][0][0] = -1 }, "neighbour"},
		{"neighbour past end", func(h *HNSW) { h.links[0][0][0] = 3 }, "neighbour"},
		{"edge to absent layer", func(h *HNSW) { h.links[0][1][0] = 2 }, "does not occupy"},
		{"degree above cap", func(h *HNSW) { h.mMax0 = 2; h.links[0][0] = []int32{1, 2, 1} }, "neighbours"},
		{"invalid M", func(h *HNSW) { h.m = 1 }, "M is"},
		{"invalid upper cap", func(h *HNSW) { h.mMax = 1 }, "at least M"},
		{"invalid lower cap", func(h *HNSW) { h.mMax0 = 1 }, "at least M"},
		{"invalid construction width", func(h *HNSW) { h.efConstruct = 1 }, "at least M"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := fixture()
			tc.mutate(h)
			got, err := LoadHNSW(bytes.NewReader(encode(t, h)))
			if err == nil || !strings.Contains(err.Error(), tc.want) || got != nil {
				t.Fatalf("got index %v, error %v; want %q", got, err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		fields map[int]uint32
	}{
		{"unknown flags", map[int]uint32{10: 4}},
		{"zero dimension", map[int]uint32{1: 0}},
		{"zero count", map[int]uint32{2: 0}},
		{"unsigned product overflow", map[int]uint32{1: 0xffffffff, 2: 0xffffffff}},
		{"signed ID overflow", map[int]uint32{1: 1, 2: 0x80000000}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := encode(t, fixture())
			for field, value := range tc.fields {
				binary.LittleEndian.PutUint32(b[len(storeMagic)+field*4:], value)
			}
			if h, err := LoadHNSW(bytes.NewReader(b)); err == nil || h != nil {
				t.Fatalf("malformed header accepted: %v", err)
			}
		})
	}
}

// Same seed, same input, same bytes. Without this a benchmark can't be repeated
// from a saved index and be trusted.
func TestSaveIsDeterministic(t *testing.T) {
	r := rand.New(rand.NewSource(52))
	set := randomSet(r, 800, 8)

	dump := func() []byte {
		h, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40, Seed: 99})
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err := h.Save(&b); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}

	if a, b := dump(), dump(); !bytes.Equal(a, b) {
		t.Errorf("two builds with the same seed produced different files, %d and %d bytes", len(a), len(b))
	}
}

func TestLoadSupportsBuildModes(t *testing.T) {
	for _, mode := range []string{"single layer", "nearest M", "parallel"} {
		t.Run(mode, func(t *testing.T) {
			p := DefaultParams()
			p.SingleLayer = mode == "single layer"
			p.NearestM = mode == "nearest M"
			if mode == "parallel" {
				p.BuildWorkers = 4
			}
			set := randomSet(rand.New(rand.NewSource(54)), 100, 8)
			built, err := BuildHNSW(set, p)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := built.Save(&buf); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadHNSW(&buf)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < set.Len(); i++ {
				want, err := built.Search(set.At(i), 10)
				if err != nil {
					t.Fatal(err)
				}
				got, err := loaded.Search(set.At(i), 10)
				if err != nil || len(got) != len(want) {
					t.Fatalf("query %d: got %v, error %v, want %v", i, got, err, want)
				}
				for j := range want {
					if got[j] != want[j] {
						t.Fatalf("query %d: got %v, want %v", i, got, want)
					}
				}
			}
		})
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	r := rand.New(rand.NewSource(53))
	h, err := BuildHNSW(randomSet(r, 200, 4), Params{M: 4, EfConstruct: 20, EfSearch: 20})
	if err != nil {
		t.Fatal(err)
	}
	var good bytes.Buffer
	if err := h.Save(&good); err != nil {
		t.Fatal(err)
	}

	corrupt := func(name string, b []byte) {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadHNSW(bytes.NewReader(b)); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}

	corrupt("empty", nil)
	corrupt("wrong magic", []byte("NOTVEC\x01\x00garbage padding here"))

	truncated := append([]byte(nil), good.Bytes()...)
	corrupt("truncated", truncated[:len(truncated)/2])

	bumped := append([]byte(nil), good.Bytes()...)
	bumped[len(storeMagic)] = 99 // version field
	corrupt("future version", bumped)
}
