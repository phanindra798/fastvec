package index

import (
	"os"
	"runtime"
	"testing"
)

// Kept from working out why ID based recall against SIFT1M came to 0.9994.
// Prints every disagreement with distances recomputed in float64. Answer was
// duplicate vectors in the dataset, see decisions.md.
//
// Opt in with FASTVEC_DIAG=1, it runs for a couple of minutes.
func TestDiagnoseSift1MMismatches(t *testing.T) {
	if os.Getenv("FASTVEC_DIAG") == "" {
		t.Skip("set FASTVEC_DIAG=1 to run")
	}

	base, queries, truth := loadSift(t, "sift")

	exact := func(a, b []float32) float64 {
		var sum float64
		for i := range a {
			d := float64(a[i]) - float64(b[i])
			sum += d * d
		}
		return sum
	}

	const k = 10
	f := NewFlat(base)
	workers := runtime.NumCPU()

	mismatches := 0
	for q := 0; q < queries.Len(); q++ {
		got, err := f.SearchN(queries.At(q), k, workers)
		if err != nil {
			t.Fatal(err)
		}

		want := truth.At(q)[:k]
		wantSet := make(map[int32]bool, k)
		for _, id := range want {
			wantSet[id] = true
		}
		gotSet := make(map[int32]bool, k)
		for _, r := range got {
			gotSet[r.ID] = true
		}

		var missed, extra []int32
		for _, id := range want {
			if !gotSet[id] {
				missed = append(missed, id)
			}
		}
		for _, r := range got {
			if !wantSet[r.ID] {
				extra = append(extra, r.ID)
			}
		}
		if len(missed) == 0 {
			continue
		}

		mismatches++
		qv := queries.At(q)
		t.Logf("query %d", q)
		for _, id := range missed {
			t.Logf("  ground truth has %7d, exact d2 = %.6f", id, exact(qv, base.At(int(id))))
		}
		for _, id := range extra {
			t.Logf("  we returned    %7d, exact d2 = %.6f", id, exact(qv, base.At(int(id))))
		}
	}

	t.Logf("queries with at least one disagreement: %d of %d", mismatches, queries.Len())
}
