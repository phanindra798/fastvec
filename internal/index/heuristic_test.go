package index

import (
	"math/rand"
	"testing"
)

// Stage 3. The heuristic has three numbers to move at once, so this measures
// all three against a build that differs only in neighbour selection.
//
//	go test -tags diagnostic -run Stage3 ./internal/index/
func TestHNSWStage3(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two 100k graphs")
	}

	base, queries, truth := loadSift100k(t)

	nearestP := DefaultParams()
	nearestP.NearestM = true

	nearest := buildTimed(t, base, nearestP, "nearest-M")
	heur := buildTimed(t, base, DefaultParams(), "heuristic")

	t.Log("nearest-M:")
	reportConnectivity(t, nearest)
	t.Log("heuristic:")
	reportConnectivity(t, heur)

	const k = 10
	for _, ef := range []int{20, 50, 100} {
		a := measure(t, nearest, base, queries, truth, ef, k)
		b := measure(t, heur, base, queries, truth, ef, k)

		t.Logf("ef=%-4d nearest-M: recall %.4f, %6.0f dist/query, %5.0f QPS",
			ef, a.recall, a.distPerQuery, a.qps)
		t.Logf("ef=%-4d heuristic: recall %.4f, %6.0f dist/query, %5.0f QPS",
			ef, b.recall, b.distPerQuery, b.qps)
	}

	// The point of the heuristic is a graph in one piece.
	count, largest := heur.Components()
	if stranded := heur.Len() - largest; stranded > heur.Len()/100 {
		t.Errorf("still %d components, %d nodes stranded", count, stranded)
	}

	heur.SetEfSearch(100)
	runGate(t, heur, 0.90)
}

// Two candidates in the same direction: the heuristic should keep the nearer
// and drop the one hiding behind it, then take one pointing elsewhere.
func TestSelectNeighboursDropsCovered(t *testing.T) {
	// node 0 at the origin. 1 and 2 both sit to the right, 2 behind 1.
	// 3 is further away than 2 but in a different direction.
	set := smallSet(t, [][]float32{
		{0, 0},
		{1, 0},
		{2, 0},
		{0, 3},
	})

	h, err := BuildHNSW(set, Params{M: 2, EfConstruct: 4, EfSearch: 4})
	if err != nil {
		t.Fatal(err)
	}

	candidates := h.rank(0, []int32{1, 2, 3})
	got := h.selectNeighbours(0, candidates, 2)

	if len(got) != 2 {
		t.Fatalf("kept %d neighbours, want 2", len(got))
	}
	if got[0].ID != 1 {
		t.Errorf("first kept is %d, want 1 (the nearest)", got[0].ID)
	}
	if got[1].ID != 3 {
		t.Errorf("second kept is %d, want 3; node 2 hides behind node 1 and should be dropped", got[1].ID)
	}
}

// With NearestM the same input keeps 1 and 2, which is what makes clusters.
func TestNearestMKeepsCovered(t *testing.T) {
	set := smallSet(t, [][]float32{
		{0, 0},
		{1, 0},
		{2, 0},
		{0, 3},
	})

	h, err := BuildHNSW(set, Params{M: 2, EfConstruct: 4, EfSearch: 4, NearestM: true})
	if err != nil {
		t.Fatal(err)
	}

	got := h.selectNeighbours(0, h.rank(0, []int32{1, 2, 3}), 2)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("got %+v, want nodes 1 and 2", got)
	}
}

func TestHeuristicBuildsOnePiece(t *testing.T) {
	r := rand.New(rand.NewSource(41))

	// Two well separated clumps. Nearest-M links inside each and never across.
	set := clumps(r, 400, 8, 2, 50)

	nearest, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40, NearestM: true})
	if err != nil {
		t.Fatal(err)
	}
	heur, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40})
	if err != nil {
		t.Fatal(err)
	}

	nc, nl := nearest.Components()
	hc, hl := heur.Components()
	t.Logf("nearest-M: %d components, largest %d of %d", nc, nl, set.Len())
	t.Logf("heuristic: %d components, largest %d of %d", hc, hl, set.Len())

	// Only the heuristic is held to this. At 800 points in two clumps,
	// nearest-M happens to stay connected too, so comparing the two here proves
	// nothing. The 100k build in TestHNSWStage3 is where the difference shows.
	if hc != 1 {
		t.Errorf("heuristic left %d components, largest %d of %d", hc, hl, set.Len())
	}
}
