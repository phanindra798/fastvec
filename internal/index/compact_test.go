package index

import (
	"bytes"
	"math/rand"
	"os"
	"reflect"
	"testing"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

func TestCompactBottomPreservesSearchAndPersistence(t *testing.T) {
	for _, workers := range []int{1, 4} {
		r := rand.New(rand.NewSource(751))
		h, err := BuildHNSW(randomSet(r, 1000, 16), Params{M: 8, EfConstruct: 40, BuildWorkers: workers})
		if err != nil {
			t.Fatal(err)
		}
		var packed bytes.Buffer
		if err := h.Save(&packed); err != nil {
			t.Fatal(err)
		}
		for i := range h.links {
			got := h.neighbours(int32(i), 0, nil)
			if !reflect.DeepEqual(got, h.links[i][0]) || cap(got) != len(got) {
				t.Fatalf("node %d adjacency mismatch", i)
			}
		}
		for _, ef := range []int{10, 100, 500} {
			h.SetEfSearch(ef)
			for q := 0; q < 100; q++ {
				v := make([]float32, 16)
				for i := range v {
					v[i] = r.Float32()
				}
				got, err := h.Search(v, 10)
				if err != nil {
					t.Fatal(err)
				}
				offsets := h.bottomOffsets
				h.bottomOffsets = nil
				want, err := h.Search(v, 10)
				h.bottomOffsets = offsets
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("workers=%d ef=%d query=%d mismatch", workers, ef, q)
				}
			}
		}
		loaded, err := LoadHNSW(bytes.NewReader(packed.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		var saved bytes.Buffer
		if err := loaded.Save(&saved); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(saved.Bytes(), packed.Bytes()) {
			t.Fatal("persistence bytes changed")
		}
	}
}

// Opt-in check of every supplied query against the original adjacency lookup.
func TestCompactDatasetResults(t *testing.T) {
	indexPath, queryPath := os.Getenv("FASTVEC_BENCH_INDEX"), os.Getenv("FASTVEC_BENCH_QUERIES")
	if testing.Short() || indexPath == "" || queryPath == "" {
		t.Skip("requires dataset paths and a non-short run")
	}
	f, err := os.Open(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	h, err := LoadHNSW(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	queries, err := fvecs.ReadFloat(queryPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, ef := range []int{100, 500} {
		h.SetEfSearch(ef)
		for i := 0; i < queries.Len(); i++ {
			got, err := h.Search(queries.At(i), 10)
			if err != nil {
				t.Fatal(err)
			}
			offsets := h.bottomOffsets
			h.bottomOffsets = nil
			want, err := h.Search(queries.At(i), 10)
			h.bottomOffsets = offsets
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("ef=%d query=%d mismatch", ef, i)
			}
		}
	}
}
