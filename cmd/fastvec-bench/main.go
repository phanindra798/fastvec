// Command fastvec-bench builds an HNSW index over a dataset, sweeps efSearch,
// and writes recall and throughput to JSON.
//
// Output matches what py/bench_baselines.py writes for hnswlib, so py/plots.py
// draws both curves on the same axes.
//
//	go run ./cmd/fastvec-bench -data /mnt/d/data/sift -name sift
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/phanindra798/fastvec/internal/benchenv"
	"github.com/phanindra798/fastvec/internal/distance"
	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/index"
	"github.com/phanindra798/fastvec/internal/topk"
)

type point struct {
	Ef         int     `json:"ef"`
	Recall     float64 `json:"recall"`
	QPS        float64 `json:"qps"`
	MsPerQuery float64 `json:"ms_per_query"`
	P50Ms      float64 `json:"p50_ms"`
	P95Ms      float64 `json:"p95_ms"`
	P99Ms      float64 `json:"p99_ms"`
}

type report struct {
	Implementation string       `json:"implementation"`
	Dataset        string       `json:"dataset"`
	BaseCount      int          `json:"base_count"`
	Dim            int          `json:"dim"`
	Queries        int          `json:"queries"`
	K              int          `json:"k"`
	M              int          `json:"M"`
	MMax0          int          `json:"MMax0"`
	EfConstruction int          `json:"ef_construction"`
	Seed           int64        `json:"seed"`
	BuildSeconds   float64      `json:"build_seconds"`
	Threads        int          `json:"threads"`
	Sweep          []point      `json:"sweep"`
	Env            benchenv.Env `json:"env"`
}

func main() {
	var (
		dataDir = flag.String("data", "/mnt/d/data/sift", "directory holding the dataset")
		name    = flag.String("name", "sift", "file prefix inside that directory")
		gtPath  = flag.String("gt", "", "ground truth path, defaults to <data>/<name>_groundtruth.ivecs")
		k       = flag.Int("k", 10, "neighbours per query")
		m       = flag.Int("M", 16, "neighbours chosen per node")
		efC     = flag.Int("efc", 200, "efConstruction")
		seed    = flag.Int64("seed", 42, "level assignment seed")
		out     = flag.String("out", "bench/results", "where to write the json")
	)
	flag.Parse()

	if err := run(*dataDir, *name, *gtPath, *k, *m, *efC, *seed, *out); err != nil {
		fmt.Fprintln(os.Stderr, "fastvec-bench:", err)
		os.Exit(1)
	}
}

func run(dir, name, gtPath string, k, m, efC int, seed int64, outDir string) error {
	base, err := fvecs.ReadFloat(filepath.Join(dir, name+"_base.fvecs"))
	if err != nil {
		return err
	}
	queries, err := fvecs.ReadFloat(filepath.Join(dir, name+"_query.fvecs"))
	if err != nil {
		return err
	}
	if gtPath == "" {
		gtPath = filepath.Join(dir, name+"_groundtruth.ivecs")
	}
	truth, err := fvecs.ReadInt(gtPath)
	if err != nil {
		return err
	}
	if truth.Len() != queries.Len() {
		return fmt.Errorf("%d ground truth rows for %d queries", truth.Len(), queries.Len())
	}
	fmt.Printf("base %d x %d, %d queries\n", base.Len(), base.Dim, queries.Len())

	p := index.Params{M: m, MMax0: 2 * m, EfConstruct: efC, EfSearch: 100, Seed: seed}

	start := time.Now()
	h, err := index.BuildHNSW(base, p)
	if err != nil {
		return err
	}
	build := time.Since(start)

	minD, maxD, meanD := h.Degrees()
	count, largest := h.Components()
	fmt.Printf("built in %v, levels %v, degree min=%d max=%d mean=%.1f\n",
		build.Round(time.Millisecond), h.LevelSizes(), minD, maxD, meanD)
	fmt.Printf("level 0 reachable sets: %d, largest %d of %d\n", count, largest, h.Len())

	rep := report{
		Implementation: "fastvec",
		Dataset:        name,
		BaseCount:      base.Len(),
		Dim:            base.Dim,
		Queries:        queries.Len(),
		K:              k,
		M:              m,
		MMax0:          2 * m,
		EfConstruction: efC,
		Seed:           seed,
		BuildSeconds:   build.Seconds(),
		Threads:        1,
		Env:            benchenv.Capture(),
	}

	// One thread, matching how hnswlib was measured. Multi-threaded numbers
	// would not be comparable to that curve.
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)

	for _, ef := range []int{10, 20, 40, 60, 80, 100, 150, 200, 300, 500} {
		if ef < k {
			continue
		}
		h.SetEfSearch(ef)

		for i := 0; i < 300 && i < queries.Len(); i++ {
			if _, err := h.Search(queries.At(i), k); err != nil {
				return err
			}
		}

		times := make([]float64, queries.Len())
		var hits int

		wall := time.Now()
		for q := 0; q < queries.Len(); q++ {
			t0 := time.Now()
			got, err := h.Search(queries.At(q), k)
			times[q] = float64(time.Since(t0).Microseconds()) / 1000
			if err != nil {
				return fmt.Errorf("query %d: %w", q, err)
			}
			hits += recall(queries.At(q), got, truth.At(q)[:k], base)
		}
		elapsed := time.Since(wall)

		sort.Float64s(times)
		pt := point{
			Ef:         ef,
			Recall:     float64(hits) / float64(queries.Len()*k),
			QPS:        float64(queries.Len()) / elapsed.Seconds(),
			MsPerQuery: 1000 * elapsed.Seconds() / float64(queries.Len()),
			P50Ms:      pct(times, 50),
			P95Ms:      pct(times, 95),
			P99Ms:      pct(times, 99),
		}
		rep.Sweep = append(rep.Sweep, pt)

		fmt.Printf("  ef=%-4d recall@%d=%.4f  %8.1f QPS  p50 %.3f  p99 %.3f ms\n",
			ef, k, pt.Recall, pt.QPS, pt.P50Ms, pt.P99Ms)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(outDir, "fastvec-"+name+".json")
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf("wrote %s\n", path)
	return nil
}

// Counts a result correct if it is at least as close as the k'th listed
// neighbour, same rule the Go tests use. SIFT1M has duplicate vectors so
// matching on ID undercounts.
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

func pct(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := (p * len(sorted)) / 100
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}
