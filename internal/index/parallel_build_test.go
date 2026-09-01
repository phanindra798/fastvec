package index

import (
	"math/rand"
	"runtime"
	"testing"
	"time"
)

// A parallel build cannot produce the same index as a sequential one, because
// the order nodes get linked in depends on scheduling. What it has to produce
// is an index just as good: same connectivity, recall within noise.
//
// Run this under -race. It is the only test that exercises concurrent writes to
// the graph.
func TestParallelBuildMatchesSequentialQuality(t *testing.T) {
	r := rand.New(rand.NewSource(71))
	set := randomSet(r, 20000, 32)

	seq, err := BuildHNSW(set, Params{M: 8, EfConstruct: 60, EfSearch: 50, Seed: 5})
	if err != nil {
		t.Fatal(err)
	}
	par, err := BuildHNSW(set, Params{M: 8, EfConstruct: 60, EfSearch: 50, Seed: 5,
		BuildWorkers: runtime.NumCPU()})
	if err != nil {
		t.Fatal(err)
	}

	sc, sl := seq.Components()
	pc, pl := par.Components()
	t.Logf("sequential: %d reachable sets, largest %d", sc, sl)
	t.Logf("parallel:   %d reachable sets, largest %d", pc, pl)

	// Not equality. A parallel build strands a few nodes, because a node
	// inserted while its neighbourhood is still half linked picks worse
	// neighbours. Measured at 100k that is 0.28% of the graph and about a third
	// of a point of recall. This test runs at 20k with every core, where a much
	// larger share of the graph is in flight at once, so the effect is worse
	// here than anywhere it would actually be used.
	if lost := float64(sl-pl) / float64(seq.Len()); lost > 0.01 {
		t.Errorf("parallel build stranded %.2f%% more nodes than sequential", 100*lost)
	}

	// Both must find the same vectors, near enough. Comparing against each
	// other rather than against ground truth keeps this about the build.
	queries := make([][]float32, 500)
	for i := range queries {
		v := make([]float32, 32)
		for j := range v {
			v[j] = r.Float32()
		}
		queries[i] = v
	}

	agree := 0
	for _, q := range queries {
		a, err := seq.Search(q, 10)
		if err != nil {
			t.Fatal(err)
		}
		b, err := par.Search(q, 10)
		if err != nil {
			t.Fatal(err)
		}

		want := make(map[int32]bool, len(a))
		for _, r := range a {
			want[r.ID] = true
		}
		for _, r := range b {
			if want[r.ID] {
				agree++
			}
		}
	}

	overlap := float64(agree) / float64(len(queries)*10)
	t.Logf("the two builds return the same neighbour %.1f%% of the time", 100*overlap)

	// Overlap is not recall. Two indexes can disagree on which of several
	// equally good neighbours to return and both be right, and at 20k with
	// every core the graphs genuinely do differ. The recall comparison at 100k
	// in TestParallelBuildRecall is the real gate; this one only catches a
	// parallel build going badly wrong.
	if overlap < 0.80 {
		t.Errorf("only %.1f%% overlap between sequential and parallel results", 100*overlap)
	}
}

// Levels are drawn up front on one goroutine, so the level assignment should
// come out identical however many workers run. Only the linking varies.
func TestParallelBuildKeepsLevelAssignment(t *testing.T) {
	r := rand.New(rand.NewSource(72))
	set := randomSet(r, 5000, 8)

	seq, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40, Seed: 11})
	if err != nil {
		t.Fatal(err)
	}
	par, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40, Seed: 11, BuildWorkers: 8})
	if err != nil {
		t.Fatal(err)
	}

	a, b := seq.LevelSizes(), par.LevelSizes()
	if len(a) != len(b) {
		t.Fatalf("sequential has %d levels, parallel has %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("level %d holds %d nodes sequentially and %d in parallel", i, a[i], b[i])
		}
	}
	t.Logf("levels identical: %v", a)
}

// A finished index takes no locks, so the search path has to be unaffected by
// the machinery a build needs.
func TestNoLocksAfterBuild(t *testing.T) {
	r := rand.New(rand.NewSource(73))
	set := randomSet(r, 2000, 16)

	h, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40, BuildWorkers: 4})
	if err != nil {
		t.Fatal(err)
	}

	if h.building {
		t.Error("index still marked as building after BuildHNSW returned")
	}
	if h.locks != nil {
		t.Error("per-node locks still allocated after the build finished")
	}
}

func TestParallelBuildRejectsNothingExtra(t *testing.T) {
	r := rand.New(rand.NewSource(74))
	set := randomSet(r, 500, 8)

	for _, workers := range []int{0, 1, 2, 3, 16, 64} {
		if _, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40,
			BuildWorkers: workers}); err != nil {
			t.Errorf("workers=%d: %v", workers, err)
		}
	}
}

func TestParallelBuildIsFaster(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two 100k graphs")
	}

	base, _, _ := loadSift100k(t)

	start := time.Now()
	seq, err := BuildHNSW(base, DefaultParams())
	if err != nil {
		t.Fatal(err)
	}
	seqTime := time.Since(start)

	p := DefaultParams()
	p.BuildWorkers = runtime.NumCPU()

	start = time.Now()
	par, err := BuildHNSW(base, p)
	if err != nil {
		t.Fatal(err)
	}
	parTime := time.Since(start)

	sc, sl := seq.Components()
	pc, pl := par.Components()

	t.Logf("sequential %v, %d sets largest %d", seqTime.Round(time.Millisecond), sc, sl)
	t.Logf("parallel   %v on %d workers, %d sets largest %d",
		parTime.Round(time.Millisecond), p.BuildWorkers, pc, pl)
	t.Logf("speedup %.2fx", float64(seqTime)/float64(parTime))

	if parTime >= seqTime {
		t.Errorf("parallel build was not faster: %v against %v", parTime, seqTime)
	}
}
