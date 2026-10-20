package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

func TestRunValidatesBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name             string
		base, queries, k int
		want             string
	}{
		{"negative base", -1, 1, 1, "positive"},
		{"zero queries", 2, 0, 1, "positive"},
		{"negative queries", 2, -1, 1, "positive"},
		{"zero k", 2, 1, 0, "k must"},
		{"k above base", 2, 1, 3, "k must"},
		{"size overflow", 2, int(^uint(0) >> 1), 2, "platform limits"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dst := filepath.Join(dir, "out")
			err := run(filepath.Join(dir, "missing"), dst, filepath.Join(dst, "gt.ivecs"), tc.base, tc.queries, tc.k, 1)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, err := os.Stat(dst); !os.IsNotExist(err) {
				t.Fatal("invalid input created output")
			}
		})
	}
}

func TestRunSubsetDimensionsAndGroundTruth(t *testing.T) {
	for _, dim := range []int{1, 2} {
		dir := t.TempDir()
		base := &fvecs.Float{Dim: 1, Data: []float32{0, 10}}
		query := &fvecs.Float{Dim: dim, Data: make([]float32, dim)}
		if err := writeFvecs(filepath.Join(dir, "sift_base.fvecs"), base); err != nil {
			t.Fatal(err)
		}
		if err := writeFvecs(filepath.Join(dir, "sift_query.fvecs"), query); err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, "out")
		gt := filepath.Join(dst, "truth.ivecs")
		err := run(dir, dst, gt, 2, 1, 2, 1)
		if dim == 2 {
			if err == nil || !strings.Contains(err.Error(), "dimension") {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, err := os.Stat(dst); !os.IsNotExist(err) {
				t.Fatal("dimension mismatch created output")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		truth, err := fvecs.ReadInt(gt)
		if err != nil {
			t.Fatal(err)
		}
		if truth.Dim != 2 || truth.Len() != 1 || truth.Data[0] != 0 || truth.Data[1] != 1 {
			t.Fatalf("incorrect ground truth: %+v", truth)
		}
	}
}
