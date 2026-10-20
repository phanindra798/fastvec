package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/index"
)

// Zero floats share the same bits as zero IDs, allowing small independent
// fvecs and ivecs fixtures without importing another command's writer.
func writeRows(t *testing.T, path string, rows [][]int32) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, row := range rows {
		if err := binary.Write(f, binary.LittleEndian, int32(len(row))); err != nil {
			t.Fatal(err)
		}
		if err := binary.Write(f, binary.LittleEndian, row); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTinyBenchmarkWritesFiniteMetrics(t *testing.T) {
	if index.CountingEnabled {
		t.Skip("diagnostic builds intentionally reject timing")
	}
	dir := t.TempDir()
	writeRows(t, filepath.Join(dir, "tiny_base.fvecs"), [][]int32{{0}, {0}})
	writeRows(t, filepath.Join(dir, "tiny_query.fvecs"), [][]int32{{0}})
	writeRows(t, filepath.Join(dir, "tiny_groundtruth.ivecs"), [][]int32{{0, 1}})
	out := filepath.Join(dir, "out")
	if err := run(dir, "tiny", "", 2, 2, 2, 42, out, ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "fastvec-tiny.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rep report
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.IndexLoadPolicy != "auto" || rep.TimingMethod != "search_only_repeated_sweeps_v2" || rep.Env.GOMAXPROCS != 1 || len(rep.Sweep) != 10 {
		t.Fatalf("unexpected metadata: %+v", rep)
	}
	for _, pt := range rep.Sweep {
		if pt.TimedSeconds < .1 || pt.TimedQueries < 1 || pt.QPS <= 0 || pt.Recall != 1 {
			t.Fatalf("invalid measurement: %+v", pt)
		}
	}
}

func TestLoadPolicyMetadata(t *testing.T) {
	if err := runWithLoadPolicy("missing", "tiny", "", 2, 2, 2, 42, "", "", "invalid"); err == nil || !strings.Contains(err.Error(), "index-load-policy") {
		t.Fatalf("invalid policy was not rejected first: %v", err)
	}
	if index.CountingEnabled {
		t.Skip("diagnostic builds reject timing")
	}
	dir := t.TempDir()
	writeRows(t, filepath.Join(dir, "tiny_base.fvecs"), [][]int32{{0}, {0}})
	writeRows(t, filepath.Join(dir, "tiny_query.fvecs"), [][]int32{{0}})
	writeRows(t, filepath.Join(dir, "tiny_groundtruth.ivecs"), [][]int32{{0, 1}})
	cache := filepath.Join(dir, "tiny.fvi")
	out := filepath.Join(dir, "out")
	for _, policy := range []string{"single-pass", "auto", "single-pass"} {
		if err := runWithLoadPolicy(dir, "tiny", "", 2, 2, 2, 42, out, cache, policy); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(out, "fastvec-tiny.json"))
		if err != nil {
			t.Fatal(err)
		}
		var rep report
		if err := json.Unmarshal(data, &rep); err != nil {
			t.Fatal(err)
		}
		if rep.IndexLoadPolicy != policy {
			t.Fatalf("policy=%q, want %q", rep.IndexLoadPolicy, policy)
		}
		for _, point := range rep.Sweep {
			if point.Recall != 1 {
				t.Fatal("load policy changed recall")
			}
		}
	}
}

func TestDiagnosticTimingRejected(t *testing.T) {
	if !index.CountingEnabled {
		t.Skip("requires diagnostic build")
	}
	dir := t.TempDir()
	writeRows(t, filepath.Join(dir, "tiny_base.fvecs"), [][]int32{{0}})
	writeRows(t, filepath.Join(dir, "tiny_query.fvecs"), [][]int32{{0}})
	writeRows(t, filepath.Join(dir, "tiny_groundtruth.ivecs"), [][]int32{{0}})
	err := run(dir, "tiny", "", 1, 2, 2, 42, filepath.Join(dir, "out"), "")
	if err == nil || !strings.Contains(err.Error(), "diagnostic") {
		t.Fatalf("got %v", err)
	}
}

func TestCachedIndexCompatibility(t *testing.T) {
	base := &fvecs.Float{Dim: 2, Data: []float32{0, 0, 1, 2, 5, 6}}
	p := index.Params{M: 2, EfConstruct: 4, EfSearch: 4, Seed: 42}
	path := filepath.Join(t.TempDir(), "tiny.fvi")
	built, _, err := loadOrBuild(base, p, path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*fvecs.Float, *index.Params)
		want   string
	}{
		{"same", func(*fvecs.Float, *index.Params) {}, ""},
		{"explicit defaults", func(_ *fvecs.Float, p *index.Params) { p.MMax, p.MMax0 = 2, 4 }, ""},
		{"query ef changes", func(_ *fvecs.Float, p *index.Params) { p.EfSearch = 100 }, ""},
		{"dimension", func(s *fvecs.Float, _ *index.Params) { s.Dim = 1 }, "shape"},
		{"count", func(s *fvecs.Float, _ *index.Params) { s.Data = s.Data[:4] }, "shape"},
		{"value", func(s *fvecs.Float, _ *index.Params) { s.Data[5] = 7 }, "dataset differs"},
		{"order", func(s *fvecs.Float, _ *index.Params) {
			s.Data[0], s.Data[2] = s.Data[2], s.Data[0]
			s.Data[1], s.Data[3] = s.Data[3], s.Data[1]
		}, "dataset differs"},
		{"signed zero", func(s *fvecs.Float, _ *index.Params) { s.Data[0] = math.Float32frombits(0x80000000) }, "dataset differs"},
		{"M", func(_ *fvecs.Float, p *index.Params) { p.M = 3 }, "M="},
		{"upper cap", func(_ *fvecs.Float, p *index.Params) { p.MMax = 3 }, "MMax="},
		{"lower cap", func(_ *fvecs.Float, p *index.Params) { p.MMax0 = 5 }, "MMax0="},
		{"construction width", func(_ *fvecs.Float, p *index.Params) { p.EfConstruct = 5 }, "efConstruction="},
		{"seed", func(_ *fvecs.Float, p *index.Params) { p.Seed++ }, "seed="},
		{"single layer", func(_ *fvecs.Float, p *index.Params) { p.SingleLayer = true }, "modes"},
		{"nearest M", func(_ *fvecs.Float, p *index.Params) { p.NearestM = true }, "modes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := &fvecs.Float{Dim: base.Dim, Data: append([]float32(nil), base.Data...)}
			requested := p
			tc.change(set, &requested)
			loaded, duration, err := loadOrBuild(set, requested, path)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "-index") || loaded != nil {
					t.Fatalf("expected mismatch %q, got %v", tc.want, err)
				}
			} else {
				if err != nil || duration != 0 {
					t.Fatalf("cache was not reused: %v, %v", duration, err)
				}
				want, err := built.Search([]float32{5, 6}, 3)
				if err != nil {
					t.Fatal(err)
				}
				got, err := loaded.Search([]float32{5, 6}, 3)
				if err != nil || len(got) != len(want) {
					t.Fatalf("search changed: %v, %v", got, err)
				}
				for i := range want {
					if got[i] != want[i] {
						t.Fatalf("search changed: %v vs %v", got, want)
					}
				}
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original, after) {
				t.Fatal("existing cache was overwritten")
			}
		})
	}
}

func TestRunRejectsInvalidFlags(t *testing.T) {
	for _, tc := range []struct {
		k, m, ef int
		want     string
	}{
		{0, 2, 2, "k must"}, {501, 2, 2, "k must"},
		{1, 1, 2, "M must"}, {1, int(^uint(0) >> 1), 2, "M must"},
		{1, 2, 1, "efConstruction"},
	} {
		dir := t.TempDir()
		err := run(dir, "tiny", "", tc.k, tc.m, tc.ef, 42, filepath.Join(dir, "out"), "")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestRunValidatesDatasetBeforeBuilding(t *testing.T) {
	for _, tc := range []struct {
		name         string
		query, truth [][]int32
		k            int
		want         string
	}{
		{"dimension mismatch", [][]int32{{0, 0}}, [][]int32{{0}}, 1, "dimension"},
		{"negative ID", [][]int32{{0}}, [][]int32{{-1}}, 1, "invalid ID"},
		{"ID past end", [][]int32{{0}}, [][]int32{{2}}, 1, "invalid ID"},
		{"short truth", [][]int32{{0}}, [][]int32{{0}}, 2, "ground truth width"},
		{"k above base", [][]int32{{0}}, [][]int32{{0, 1, 0}}, 3, "base count"},
		{"row mismatch", [][]int32{{0}, {0}}, [][]int32{{0}}, 1, "ground truth rows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRows(t, filepath.Join(dir, "tiny_base.fvecs"), [][]int32{{0}, {0}})
			writeRows(t, filepath.Join(dir, "tiny_query.fvecs"), tc.query)
			writeRows(t, filepath.Join(dir, "tiny_groundtruth.ivecs"), tc.truth)
			out, idx := filepath.Join(dir, "out"), filepath.Join(dir, "tiny.fvi")
			err := run(dir, "tiny", "", tc.k, 2, 2, 42, out, idx)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, path := range []string{out, idx} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("invalid input created %s", path)
				}
			}
		})
	}
}

func TestValidateDatasetAcceptsBoundaryK(t *testing.T) {
	base := &fvecs.Float{Dim: 1, Data: []float32{0, 10}}
	queries := &fvecs.Float{Dim: 1, Data: []float32{0}}
	truth := &fvecs.Int{Dim: 2, Data: []int32{0, 1}}
	for _, k := range []int{1, 2} {
		if err := validateDataset(base, queries, truth, k); err != nil {
			t.Fatalf("valid k=%d rejected: %v", k, err)
		}
	}
}
