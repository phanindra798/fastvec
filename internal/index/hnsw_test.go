package index

import (
	"math/rand"
	"testing"
	"time"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

func buildFromSift100k(t *testing.T, p Params) *HNSW {
	t.Helper()

	base, _, _ := loadSift100k(t)

	start := time.Now()
	h, err := BuildHNSW(base, p)
	if err != nil {
		t.Fatal(err)
	}
	min, max, mean := h.Degrees()
	t.Logf("built %d nodes in %v, degree min=%d max=%d mean=%.1f",
		h.Len(), time.Since(start).Round(time.Millisecond), min, max, mean)

	return h
}

// Stage 1 gate. Set low because a flat graph with the naive neighbour choice
// only has to work, not be good.
func TestHNSWStage1(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 100k graph")
	}

	h := buildFromSift100k(t, DefaultParams())
	reportConnectivity(t, h)
	runGate(t, h, 0.70)
}

// Reports whether the level-0 graph is in one piece.
//
// Pruning a neighbour list can drop the only edge between two regions. Once the
// graph splits, a query whose descent lands in one piece can never reach
// vectors in another, whatever efSearch is set to.
//
// Fragmentation is logged rather than failed. Nearest-M selection produces it
// by design and stage 3's heuristic is the fix, so the number is here to be
// compared before and after. Only a graph in pieces smaller than half the
// dataset counts as broken.
func reportConnectivity(t *testing.T, h *HNSW) {
	t.Helper()

	count, largest := h.Components()
	stranded := h.Len() - largest
	t.Logf("level 0: %d components, largest %d of %d, %d stranded (%.1f%%)",
		count, largest, h.Len(), stranded, 100*float64(stranded)/float64(h.Len()))

	if float64(largest) < 0.5*float64(h.Len()) {
		t.Errorf("largest component is only %d of %d nodes", largest, h.Len())
	}
}

// efSearch is the accuracy dial, so recall has to climb with it. If this stays
// flat the candidate list is not doing anything.
func TestEfSearchRaisesRecall(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 100k graph")
	}

	h := buildFromSift100k(t, DefaultParams())
	base, queries, truth := loadSift100k(t)

	const k = 10
	prev := 0.0

	for _, ef := range []int{10, 20, 50, 100, 200} {
		h.SetEfSearch(ef)

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

		r := float64(hits) / float64(total)
		t.Logf("ef=%-4d recall@%d=%.4f  %.0f QPS", ef, k, r,
			float64(queries.Len())/elapsed.Seconds())

		if r < prev-0.005 {
			t.Errorf("ef=%d gave recall %.4f, lower than the previous %.4f", ef, r, prev)
		}
		prev = r
	}
}

// Small enough to reason about, and it catches the obvious build failures
// without waiting for a 100k graph.
func TestHNSWSmallGraph(t *testing.T) {
	r := rand.New(rand.NewSource(31))
	set := randomSet(r, 500, 8)

	h, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40})
	if err != nil {
		t.Fatal(err)
	}

	reportConnectivity(t, h)

	_, max, _ := h.Degrees()
	if max > 16 {
		t.Errorf("a node has %d neighbours, MMax is 16", max)
	}

	// Every stored vector is its own nearest neighbour at distance zero, so a
	// search for one should find it.
	misses := 0
	for id := 0; id < set.Len(); id++ {
		got, err := h.Search(set.At(id), 1)
		if err != nil {
			t.Fatal(err)
		}
		if got[0].Dist != 0 {
			misses++
		}
	}
	if misses > set.Len()/20 {
		t.Errorf("%d of %d vectors could not find themselves", misses, set.Len())
	}
}

func TestHNSWRejectsBadInput(t *testing.T) {
	r := rand.New(rand.NewSource(32))
	set := randomSet(r, 50, 4)

	if _, err := BuildHNSW(set, Params{M: 0, EfConstruct: 10}); err == nil {
		t.Error("M of 0 was accepted")
	}
	if _, err := BuildHNSW(set, Params{M: 16, EfConstruct: 4}); err == nil {
		t.Error("efConstruction below M was accepted")
	}
	if _, err := BuildHNSW(set, Params{M: 16, MMax: 4, EfConstruct: 40}); err == nil {
		t.Error("MMax below M was accepted")
	}

	// Used to build fine and then panic inside Search, because entry pointed at
	// node 0 of an empty index.
	if _, err := BuildHNSW(&fvecs.Float{Dim: 4}, DefaultParams()); err == nil {
		t.Error("empty set was accepted")
	}
}

// One vector is a legitimate index: it's its own nearest neighbour and there is
// nothing to walk to.
func TestHNSWSingleVector(t *testing.T) {
	h, err := BuildHNSW(&fvecs.Float{Dim: 2, Data: []float32{1, 2}}, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}

	got, err := h.Search([]float32{1, 2}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 0 || got[0].Dist != 0 {
		t.Errorf("got %+v, want one result, ID 0, distance 0", got)
	}
}

// Tuning the dial while queries are in flight is a reasonable thing for a
// server to do, and efSearch used to be a plain int. -race never caught it
// because nothing did both at once.
func TestSetEfSearchDuringSearch(t *testing.T) {
	r := rand.New(rand.NewSource(34))
	set := randomSet(r, 1000, 8)

	h, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40})
	if err != nil {
		t.Fatal(err)
	}

	q := make([]float32, 8)
	for i := range q {
		q[i] = r.Float32()
	}

	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		for ef := 10; ; ef = 10 + (ef+7)%200 {
			select {
			case <-stop:
				close(done)
				return
			default:
				h.SetEfSearch(ef)
			}
		}
	}()

	for i := 0; i < 2000; i++ {
		if _, err := h.Search(q, 10); err != nil {
			t.Fatal(err)
		}
	}

	close(stop)
	<-done
}

func TestHNSWConcurrentSearchMatchesSerial(t *testing.T) {
	r := rand.New(rand.NewSource(33))
	set := randomSet(r, 2000, 16)

	h, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40})
	if err != nil {
		t.Fatal(err)
	}

	queries := make([][]float32, 200)
	for i := range queries {
		v := make([]float32, 16)
		for j := range v {
			v[j] = r.Float32()
		}
		queries[i] = v
	}

	want := make([][]int32, len(queries))
	for i, q := range queries {
		got, err := h.Search(q, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, res := range got {
			want[i] = append(want[i], res.ID)
		}
	}

	done := make(chan int, 16)
	for w := 0; w < 16; w++ {
		go func() {
			bad := 0
			for i, q := range queries {
				got, err := h.Search(q, 10)
				if err != nil {
					bad++
					continue
				}
				for j, res := range got {
					if j >= len(want[i]) || res.ID != want[i][j] {
						bad++
						break
					}
				}
			}
			done <- bad
		}()
	}
	for w := 0; w < 16; w++ {
		if bad := <-done; bad != 0 {
			t.Errorf("worker saw %d queries disagree with the serial answer", bad)
		}
	}
}
