package index

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"reflect"
	"testing"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

func TestWorkspaceEpochLifecycle(t *testing.T) {
	w := newWorkspace(8)
	w.begin(8, 4)
	w.mark(3)
	if !w.seen(3) {
		t.Fatal("mark not visible")
	}
	w.begin(8, 4)
	if w.seen(3) {
		t.Fatal("previous search remains visible")
	}
	for i := range w.stamp {
		w.stamp[i] = 1
	}
	w.epoch = 0
	w.epoch--     // Maximum value for either stamp width.
	w.begin(4, 4) // Clearing must include retained slots beyond the current size.
	if w.epoch != 1 {
		t.Fatalf("wrapped epoch=%d", w.epoch)
	}
	for i := range w.stamp {
		if w.seen(int32(i)) {
			t.Fatalf("stale mark at %d", i)
		}
	}
	w.mark(2)
	w.begin(16, 4)
	if len(w.stamp) != 16 || w.seen(2) {
		t.Fatal("growth retained stale marks")
	}
	w.mark(15)
	if !w.seen(15) {
		t.Fatal("new slot cannot be marked")
	}
}

func TestSearchLayerAfterEpochRollover(t *testing.T) {
	h, err := BuildHNSW(randomSet(rand.New(rand.NewSource(731)), 300, 16), DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	w := newWorkspace(h.Len())
	for _, ef := range []int{10, 100, 300} {
		for q := 0; q < 20; q++ {
			v := h.vec(int32(q))
			want := h.searchLayer(v, h.entry, ef, 0, -1, newWorkspace(h.Len()))
			for i := range w.stamp {
				w.stamp[i] = 1
			}
			w.epoch = 0
			w.epoch-- // Maximum value for either stamp width.
			got := h.searchLayer(v, h.entry, ef, 0, -1, w)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ef=%d q=%d rollover changes results", ef, q)
			}
		}
	}
}

func BenchmarkWorkspaceEpoch(b *testing.B) {
	for _, mode := range []string{"amortized", "rollover"} {
		b.Run(mode, func(b *testing.B) {
			w := newWorkspace(1_000_000)
			w.begin(1_000_000, 100)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if mode == "rollover" {
					w.epoch = 0
					w.epoch-- // Maximum value for either stamp width.
				}
				w.begin(1_000_000, 100)
				w.mark(int32(i % 1_000_000))
			}
		})
	}
}

// Opt-in fingerprint lets separate binaries compare exact IDs and float32 bits.
func TestSearchDatasetFingerprint(t *testing.T) {
	indexPath, queriesPath := os.Getenv("FASTVEC_BENCH_INDEX"), os.Getenv("FASTVEC_BENCH_QUERIES")
	if testing.Short() || indexPath == "" || queriesPath == "" {
		t.Skip("requires dataset paths and a non-short run")
	}
	f, err := os.Open(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h, err := LoadHNSW(f)
	if err != nil {
		t.Fatal(err)
	}
	queries, err := fvecs.ReadFloat(queriesPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, ef := range []int{100, 500} {
		h.SetEfSearch(ef)
		digest := sha256.New()
		var record [8]byte
		for q := 0; q < queries.Len(); q++ {
			got, err := h.Search(queries.At(q), 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 10 {
				t.Fatalf("query %d returned %d", q, len(got))
			}
			for _, r := range got {
				binary.LittleEndian.PutUint32(record[:4], uint32(r.ID))
				binary.LittleEndian.PutUint32(record[4:], math.Float32bits(r.Dist))
				digest.Write(record[:])
			}
		}
		fmt.Printf("FINGERPRINT ef=%d queries=%d sha256=%x\n", ef, queries.Len(), digest.Sum(nil))
	}
}
