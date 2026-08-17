package index

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/phanindra798/fastvec/internal/distance"
	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/topk"
)

// dataDir points at wherever the SIFT files live. Overridable so this isn't
// pinned to my machine.
func dataDir() string {
	if d := os.Getenv("FASTVEC_DATA"); d != "" {
		return d
	}
	return "/mnt/d/data"
}

// recall counts a result correct if it's at least as close as the k'th
// neighbour the dataset lists, rather than requiring the same ID. SIFT1M has
// duplicate vectors, so several IDs tie exactly. See decisions.md.
//
// Threshold distances use the same float32 function as the search, so the
// comparison is self-consistent.
func recall(q []float32, got []topk.Result, want []int32, base *fvecs.Float) int {
	var threshold float32
	for _, id := range want {
		if d := distance.L2Squared(q, base.At(int(id))); d > threshold {
			threshold = d
		}
	}

	hits := 0
	for _, r := range got {
		if r.Dist <= threshold {
			hits++
		}
	}
	return hits
}

// Both datasets name their files after the directory, so "sift" gives
// sift/sift_base.fvecs and "siftsmall" gives siftsmall/siftsmall_base.fvecs.
func loadSift(t *testing.T, name string) (base, queries *fvecs.Float, truth *fvecs.Int) {
	t.Helper()

	dir := filepath.Join(dataDir(), name)
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("no dataset at %s", dir)
	}

	var err error
	if base, err = fvecs.ReadFloat(filepath.Join(dir, name+"_base.fvecs")); err != nil {
		t.Fatal(err)
	}
	if queries, err = fvecs.ReadFloat(filepath.Join(dir, name+"_query.fvecs")); err != nil {
		t.Fatal(err)
	}
	if truth, err = fvecs.ReadInt(filepath.Join(dir, name+"_groundtruth.ivecs")); err != nil {
		t.Fatal(err)
	}

	if base.Dim != queries.Dim {
		t.Fatalf("base is %d dimensional, queries are %d", base.Dim, queries.Dim)
	}
	if truth.Len() != queries.Len() {
		t.Fatalf("%d ground truth rows for %d queries", truth.Len(), queries.Len())
	}
	return base, queries, truth
}

// Flat is exact, so it has to match the ground truth the dataset ships with.
// One number covering the reader, the distance function, the heap and the
// tie-break at once.
func TestFlatMatchesSiftSmallGroundTruth(t *testing.T) {
	base, queries, truth := loadSift(t, "siftsmall")
	t.Logf("base %d x %d, %d queries", base.Len(), base.Dim, queries.Len())

	const k = 10
	f := NewFlat(base)

	var hits, total int
	for q := 0; q < queries.Len(); q++ {
		got, err := f.Search(queries.At(q), k)
		if err != nil {
			t.Fatalf("query %d: %v", q, err)
		}
		hits += recall(queries.At(q), got, truth.At(q)[:k], base)
		total += k
	}

	r := float64(hits) / float64(total)
	t.Logf("recall@%d: %.4f", k, r)
	if r != 1.0 {
		t.Errorf("exact search should be perfect, got %.4f", r)
	}
}

// Same check at full size: 10,000 queries over a million vectors. Minutes even
// across all cores, so -short skips it.
func TestFlatMatchesSift1MGroundTruth(t *testing.T) {
	if testing.Short() {
		t.Skip("takes minutes")
	}

	load := time.Now()
	base, queries, truth := loadSift(t, "sift")
	t.Logf("loaded %d x %d base and %d queries in %v",
		base.Len(), base.Dim, queries.Len(), time.Since(load).Round(time.Millisecond))

	const k = 10
	workers := runtime.NumCPU()
	f := NewFlat(base)

	var hits, total int
	start := time.Now()

	for q := 0; q < queries.Len(); q++ {
		got, err := f.SearchN(queries.At(q), k, workers)
		if err != nil {
			t.Fatalf("query %d: %v", q, err)
		}
		hits += recall(queries.At(q), got, truth.At(q)[:k], base)
		total += k
	}

	elapsed := time.Since(start)
	r := float64(hits) / float64(total)

	t.Logf("%d queries in %v, %v each, %.1f QPS on %d workers",
		queries.Len(), elapsed.Round(time.Second),
		(elapsed / time.Duration(queries.Len())).Round(time.Microsecond),
		float64(queries.Len())/elapsed.Seconds(), workers)
	t.Logf("recall@%d: %.4f", k, r)

	if r != 1.0 {
		t.Errorf("exact search should be perfect, got %.4f", r)
	}
}
