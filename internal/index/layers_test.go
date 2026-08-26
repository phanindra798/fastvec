package index

import (
	"github.com/phanindra798/fastvec/internal/fvecs"
	"testing"
	"time"
)

// Stage 2 gate, plus the comparison that says whether layers actually did
// anything.
//
// Recall alone can't answer that. A hierarchy is supposed to reach the same
// answer by touching fewer nodes, and on a laptop a wall-clock difference could
// just be the machine. Distance count is the honest measure, so this needs a
// diagnostic build:
//
//	go test -tags diagnostic -run Stage2 ./internal/index/
func TestHNSWStage2(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two 100k graphs")
	}

	base, queries, truth := loadSift100k(t)

	flatP := DefaultParams()
	flatP.SingleLayer = true

	flat := buildTimed(t, base, flatP, "single layer")
	layered := buildTimed(t, base, DefaultParams(), "layered")

	t.Logf("layers: maxLevel=%d sizes=%v", layered.MaxLevel(), layered.LevelSizes())

	if layered.MaxLevel() < 1 {
		t.Fatal("layered build produced only one level")
	}

	// Both, because the difference is the interesting number. Inserting through
	// a greedy descent keeps new links local, where the flat build searched the
	// whole graph from one entry point every time.
	t.Log("single layer:")
	reportConnectivity(t, flat)
	t.Log("layered:")
	reportConnectivity(t, layered)

	const k = 10
	for _, ef := range []int{20, 50, 100} {
		a := measure(t, flat, base, queries, truth, ef, k)
		b := measure(t, layered, base, queries, truth, ef, k)

		t.Logf("ef=%-4d single: recall %.4f, %6.0f dist/query, %5.0f QPS",
			ef, a.recall, a.distPerQuery, a.qps)
		t.Logf("ef=%-4d layered: recall %.4f, %6.0f dist/query, %5.0f QPS",
			ef, b.recall, b.distPerQuery, b.qps)

		if CountingEnabled && b.recall >= a.recall-0.005 && b.distPerQuery > a.distPerQuery {
			t.Errorf("ef=%d: layers cost more distances (%.0f vs %.0f) without buying recall",
				ef, b.distPerQuery, a.distPerQuery)
		}
	}

	layered.SetEfSearch(100)
	runGate(t, layered, 0.85)
}

type measured struct {
	recall       float64
	qps          float64
	distPerQuery float64
}

func measure(t *testing.T, h *HNSW, base, queries *fvecs.Float, truth *fvecs.Int, ef, k int) measured {
	t.Helper()

	h.SetEfSearch(ef)
	h.ResetDistances()

	var hits, total int
	start := time.Now()
	for q := 0; q < queries.Len(); q++ {
		got, err := h.Search(queries.At(q), k)
		if err != nil {
			t.Fatal(err)
		}
		hits += recall(queries.At(q), got, truth.At(q)[:k], base)
		total += k
	}
	elapsed := time.Since(start)

	return measured{
		recall:       float64(hits) / float64(total),
		qps:          float64(queries.Len()) / elapsed.Seconds(),
		distPerQuery: float64(h.Distances()) / float64(queries.Len()),
	}
}

func buildTimed(t *testing.T, base *fvecs.Float, p Params, label string) *HNSW {
	t.Helper()

	start := time.Now()
	h, err := BuildHNSW(base, p)
	if err != nil {
		t.Fatal(err)
	}
	min, max, mean := h.Degrees()
	t.Logf("%s: built in %v, degree min=%d max=%d mean=%.1f",
		label, time.Since(start).Round(time.Millisecond), min, max, mean)
	return h
}
