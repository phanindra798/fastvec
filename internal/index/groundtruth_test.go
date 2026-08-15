package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

// dataDir points at wherever the SIFT files live. Overridable so this isn't
// pinned to my machine.
func dataDir() string {
	if d := os.Getenv("FASTVEC_DATA"); d != "" {
		return d
	}
	return "/mnt/d/data"
}

// Flat is exact, so its top k has to match the ground truth the dataset ships
// with. If this passes, the reader, the distance function, the heap and the
// tie-break are all correct together. If it fails, the shape of the mismatch
// says which one.
//
// Skips when the data isn't downloaded, so CI stays green without it.
func TestFlatMatchesSiftSmallGroundTruth(t *testing.T) {
	dir := filepath.Join(dataDir(), "siftsmall")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("no dataset at %s, run make data", dir)
	}

	base, err := fvecs.ReadFloat(filepath.Join(dir, "siftsmall_base.fvecs"))
	if err != nil {
		t.Fatal(err)
	}
	queries, err := fvecs.ReadFloat(filepath.Join(dir, "siftsmall_query.fvecs"))
	if err != nil {
		t.Fatal(err)
	}
	truth, err := fvecs.ReadInt(filepath.Join(dir, "siftsmall_groundtruth.ivecs"))
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("base %d x %d, queries %d, ground truth %d x %d",
		base.Len(), base.Dim, queries.Len(), truth.Len(), truth.Dim)

	if base.Dim != queries.Dim {
		t.Fatalf("base is %d dimensional, queries are %d", base.Dim, queries.Dim)
	}
	if truth.Len() != queries.Len() {
		t.Fatalf("%d ground truth rows for %d queries", truth.Len(), queries.Len())
	}

	const k = 10
	f := NewFlat(base)

	var hits, total int
	mismatched := 0

	for q := 0; q < queries.Len(); q++ {
		got, err := f.Search(queries.At(q), k)
		if err != nil {
			t.Fatalf("query %d: %v", q, err)
		}

		want := truth.At(q)[:k]
		wantSet := make(map[int32]bool, k)
		for _, id := range want {
			wantSet[id] = true
		}

		for _, r := range got {
			if wantSet[r.ID] {
				hits++
			}
		}
		total += k

		// Report the first few disagreements in full. Distances are printed
		// because equal ones mean a legitimate tie rather than a bug.
		if got[0].ID != want[0] && mismatched < 3 {
			mismatched++
			t.Errorf("query %d nearest: got id %d at %v, want id %d",
				q, got[0].ID, got[0].Dist, want[0])
		}
	}

	recall := float64(hits) / float64(total)
	t.Logf("recall@%d against shipped ground truth: %.4f", k, recall)

	if recall != 1.0 {
		t.Errorf("exact search should match ground truth perfectly, got %.4f", recall)
	}
}
