package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

// The 100k subset lives with the other datasets, but its ground truth is
// committed, so the two come from different places.
func loadSift100k(t *testing.T) (base, queries *fvecs.Float, truth *fvecs.Int) {
	t.Helper()

	dir := filepath.Join(dataDir(), "sift100k")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("no dataset at %s, run: go run ./cmd/fastvec-subset", dir)
	}

	var err error
	if base, err = fvecs.ReadFloat(filepath.Join(dir, "sift100k_base.fvecs")); err != nil {
		t.Fatal(err)
	}
	if queries, err = fvecs.ReadFloat(filepath.Join(dir, "sift100k_query.fvecs")); err != nil {
		t.Fatal(err)
	}
	if truth, err = fvecs.ReadInt("../../bench/data/sift100k_groundtruth.ivecs"); err != nil {
		t.Fatal(err)
	}
	return base, queries, truth
}

// runGate measures recall@10 for any index against the 100k subset and fails if
// it comes in under want.
//
// Each HNSW stage gets one of these with a different threshold, so the
// measurement is identical across stages and only the bar moves. Timing is
// logged but never asserted on: a test machine under load would fail a
// perfectly good index.
func runGate(t *testing.T, idx Index, want float64) {
	t.Helper()

	base, queries, truth := loadSift100k(t)
	if idx.Len() != base.Len() {
		t.Fatalf("index holds %d vectors, dataset has %d", idx.Len(), base.Len())
	}

	const k = 10
	var hits, total int

	start := time.Now()
	for q := 0; q < queries.Len(); q++ {
		got, err := idx.Search(queries.At(q), k)
		if err != nil {
			t.Fatalf("query %d: %v", q, err)
		}
		hits += recall(queries.At(q), got, truth.At(q)[:k], base)
		total += k
	}
	elapsed := time.Since(start)

	r := float64(hits) / float64(total)
	perQuery := elapsed / time.Duration(queries.Len())

	t.Logf("recall@%d = %.4f over %d queries, %v each, %.0f QPS",
		k, r, queries.Len(), perQuery.Round(time.Microsecond),
		float64(queries.Len())/elapsed.Seconds())

	if r < want {
		t.Errorf("recall@%d = %.4f, gate is %.2f", k, r, want)
	}
}

// Flat is exact, so it should sail through any threshold. This also proves the
// harness itself works before an approximate index depends on it.
//
// Skipped under -short. 1000 queries scanning 100k vectors each takes 11s on
// its own and over three minutes under the race detector.
func TestGateHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("1000 full scans, too slow for the short suite")
	}
	base, _, _ := loadSift100k(t)
	runGate(t, NewFlat(base), 1.0)
}
