package index

import (
	"github.com/phanindra798/fastvec/internal/fvecs"
	"runtime"
	"testing"
	"time"
)

// The trade a parallel build makes, measured. Speed against graph quality, and
// the only quality number that matters is recall.
//
//	go test -run ParallelBuildRecall -v ./internal/index/
func TestParallelBuildRecall(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two 100k graphs")
	}

	base, queries, truth := loadSift100k(t)

	seqP := DefaultParams()
	parP := DefaultParams()
	parP.BuildWorkers = runtime.NumCPU()

	seq := timedBuild(t, base, seqP, "sequential")
	par := timedBuild(t, base, parP, "parallel")

	const k = 10
	for _, ef := range []int{20, 100, 200} {
		a := measure(t, seq, base, queries, truth, ef, k)
		b := measure(t, par, base, queries, truth, ef, k)

		t.Logf("ef=%-4d sequential recall %.4f, %5.0f QPS", ef, a.recall, a.qps)
		t.Logf("ef=%-4d parallel   recall %.4f, %5.0f QPS", ef, b.recall, b.qps)

		if b.recall < a.recall-0.01 {
			t.Errorf("ef=%d: parallel build cost %.4f recall, more than a point",
				ef, a.recall-b.recall)
		}
	}
}

func timedBuild(t *testing.T, base *fvecs.Float, p Params, label string) *HNSW {
	t.Helper()

	start := time.Now()
	h, err := BuildHNSW(base, p)
	if err != nil {
		t.Fatal(err)
	}
	count, largest := h.Components()
	t.Logf("%s: built in %v, %d reachable sets, largest %d of %d",
		label, time.Since(start).Round(time.Millisecond), count, largest, h.Len())
	return h
}
