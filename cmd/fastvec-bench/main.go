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
	TimedQueries int64   `json:"timed_queries"`
	TimedSeconds float64 `json:"timed_seconds"`
	Ef           int     `json:"ef"`
	Recall       float64 `json:"recall"`
	QPS          float64 `json:"qps"`
	MsPerQuery   float64 `json:"ms_per_query"`
	P50Ms        float64 `json:"p50_ms"`
	P95Ms        float64 `json:"p95_ms"`
	P99Ms        float64 `json:"p99_ms"`
}

type report struct {
	IndexLoadPolicy string       `json:"index_load_policy"`
	TimingMethod    string       `json:"timing_method"`
	AVX2            bool         `json:"avx2"`
	Implementation  string       `json:"implementation"`
	Dataset         string       `json:"dataset"`
	BaseCount       int          `json:"base_count"`
	Dim             int          `json:"dim"`
	Queries         int          `json:"queries"`
	K               int          `json:"k"`
	M               int          `json:"M"`
	MMax0           int          `json:"MMax0"`
	EfConstruction  int          `json:"ef_construction"`
	Seed            int64        `json:"seed"`
	BuildSeconds    float64      `json:"build_seconds"`
	Threads         int          `json:"threads"`
	Sweep           []point      `json:"sweep"`
	Env             benchenv.Env `json:"env"`
}

func main() {
	var (
		dataDir    = flag.String("data", "/mnt/d/data/sift", "directory holding the dataset")
		name       = flag.String("name", "sift", "file prefix inside that directory")
		gtPath     = flag.String("gt", "", "ground truth path, defaults to <data>/<name>_groundtruth.ivecs")
		k          = flag.Int("k", 10, "neighbours per query")
		m          = flag.Int("M", 16, "neighbours chosen per node")
		efC        = flag.Int("efc", 200, "efConstruction")
		seed       = flag.Int64("seed", 42, "level assignment seed")
		out        = flag.String("out", "bench/results", "where to write the json")
		idxPath    = flag.String("index", "", "load the index from here if it exists, otherwise build and save it")
		loadPolicy = flag.String("index-load-policy", "auto", "cached index loading: auto or single-pass (ignored when building)")
	)
	flag.Parse()

	if err := runWithLoadPolicy(*dataDir, *name, *gtPath, *k, *m, *efC, *seed, *out, *idxPath, *loadPolicy); err != nil {
		fmt.Fprintln(os.Stderr, "fastvec-bench:", err)
		os.Exit(1)
	}
}

func run(dir, name, gtPath string, k, m, efC int, seed int64, outDir, idxPath string) error {
	return runWithLoadPolicy(dir, name, gtPath, k, m, efC, seed, outDir, idxPath, "auto")
}

func runWithLoadPolicy(dir, name, gtPath string, k, m, efC int, seed int64, outDir, idxPath, loadPolicy string) error {
	if loadPolicy != "auto" && loadPolicy != "single-pass" {
		return fmt.Errorf("index-load-policy must be auto or single-pass, got %q", loadPolicy)
	}
	if k < 1 || k > 500 {
		return fmt.Errorf("k must be between 1 and maximum sweep ef 500, got %d", k)
	}
	if m < 2 || m > int(^uint(0)>>1)/2 {
		return fmt.Errorf("M must be at least 2 and small enough for 2*M, got %d", m)
	}
	if efC < m {
		return fmt.Errorf("efConstruction (%d) must be at least M (%d)", efC, m)
	}
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
	if err := validateDataset(base, queries, truth, k); err != nil {
		return err
	}
	if index.CountingEnabled {
		return fmt.Errorf("timing benchmarks require a build without the diagnostic tag")
	}
	fmt.Printf("base %d x %d, %d queries\n", base.Len(), base.Dim, queries.Len())

	p := index.Params{M: m, MMax0: 2 * m, EfConstruct: efC, EfSearch: 100, Seed: seed}

	h, build, err := loadOrBuildWithOptions(base, p, idxPath, index.LoadOptions{SinglePass: loadPolicy == "single-pass"})
	if err != nil {
		return err
	}

	minD, maxD, meanD := h.Degrees()
	count, largest := h.Components()
	origin := "loaded from disk"
	if build > 0 {
		origin = "built in " + build.Round(time.Millisecond).String()
	}
	fmt.Printf("%s, levels %v, degree min=%d max=%d mean=%.1f\n",
		origin, h.LevelSizes(), minD, maxD, meanD)
	fmt.Printf("level 0 reachable sets: %d, largest %d of %d\n", count, largest, h.Len())

	// One thread, matching how hnswlib was measured. Multi-threaded numbers
	// would not be comparable to that curve.
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)

	rep := report{
		IndexLoadPolicy: loadPolicy,
		TimingMethod:    "search_only_repeated_sweeps_v2",
		AVX2:            distance.UsingAVX2(),
		Implementation:  "fastvec",
		Dataset:         name,
		BaseCount:       base.Len(),
		Dim:             base.Dim,
		Queries:         queries.Len(),
		K:               k,
		M:               m,
		MMax0:           2 * m,
		EfConstruction:  efC,
		Seed:            seed,
		BuildSeconds:    build.Seconds(),
		Threads:         1,
		Env:             benchenv.Capture(),
	}

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

		timedQueries, elapsed, err := measureThroughput(func() error {
			for q := 0; q < queries.Len(); q++ {
				if _, err := h.Search(queries.At(q), k); err != nil {
					return err
				}
			}
			return nil
		}, queries.Len(), time.Now)
		if err != nil {
			return err
		}

		// Latency and recall use a separate pass, outside throughput timing.
		for q := 0; q < queries.Len(); q++ {
			t0 := time.Now()
			got, err := h.Search(queries.At(q), k)
			times[q] = float64(time.Since(t0)) / float64(time.Millisecond)
			if err != nil {
				return fmt.Errorf("query %d: %w", q, err)
			}
			hits += recall(queries.At(q), got, truth.At(q)[:k], base)
		}

		sort.Float64s(times)
		pt := point{
			Ef:           ef,
			TimedQueries: timedQueries,
			TimedSeconds: elapsed.Seconds(),
			Recall:       float64(hits) / float64(queries.Len()*k),
			QPS:          float64(timedQueries) / elapsed.Seconds(),
			MsPerQuery:   1000 * elapsed.Seconds() / float64(timedQueries),
			P50Ms:        pct(times, 50),
			P95Ms:        pct(times, 95),
			P99Ms:        pct(times, 99),
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

// loadOrBuild reads a saved index when one is there, otherwise builds and saves
// it. A million vectors take 11 minutes to build, which is too long to pay
// again every time a timing needs re-measuring on an idle machine.
//
// The returned duration is the build time, or zero when it came off disk. A
// loaded index reports no build time rather than a fake one.
func loadOrBuild(base *fvecs.Float, p index.Params, path string) (*index.HNSW, time.Duration, error) {
	return loadOrBuildWithOptions(base, p, path, index.LoadOptions{})
}

func loadOrBuildWithOptions(base *fvecs.Float, p index.Params, path string, opts index.LoadOptions) (*index.HNSW, time.Duration, error) {
	if path != "" {
		f, err := os.Open(path)
		switch {
		case err == nil:
			defer f.Close()
			start := time.Now()
			h, err := index.LoadHNSWWithOptions(f, opts)
			if err != nil {
				return nil, 0, fmt.Errorf("%s: %w", path, err)
			}
			if err := h.ValidateReuse(base, p); err != nil {
				return nil, 0, fmt.Errorf("%s: %w; choose a different -index path or remove this cache to rebuild", path, err)
			}
			fmt.Printf("loaded index from %s in %v\n", path, time.Since(start).Round(time.Millisecond))
			return h, 0, nil
		case !os.IsNotExist(err):
			return nil, 0, err
		}
	}

	start := time.Now()
	h, err := index.BuildHNSW(base, p)
	if err != nil {
		return nil, 0, err
	}
	build := time.Since(start)

	if path != "" {
		f, err := os.Create(path)
		if err != nil {
			return nil, 0, err
		}
		defer f.Close()
		if err := h.Save(f); err != nil {
			return nil, 0, err
		}
		fmt.Printf("saved index to %s\n", path)
	}
	return h, build, nil
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

// validateDataset checks cross-file relationships after the vecs readers validate records.
func validateDataset(base, queries *fvecs.Float, truth *fvecs.Int, k int) error {
	if truth.Len() != queries.Len() {
		return fmt.Errorf("%d ground truth rows for %d queries", truth.Len(), queries.Len())
	}
	if base.Dim != queries.Dim {
		return fmt.Errorf("base dimension %d differs from query dimension %d", base.Dim, queries.Dim)
	}
	if k > base.Len() || k > truth.Dim {
		return fmt.Errorf("k (%d) exceeds base count (%d) or ground truth width (%d)", k, base.Len(), truth.Dim)
	}
	for i, id := range truth.Data {
		if id < 0 || int64(id) >= int64(base.Len()) {
			return fmt.Errorf("ground truth row %d has invalid ID %d for %d vectors", i/truth.Dim, id, base.Len())
		}
	}
	return nil
}
