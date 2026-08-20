// Command fastvec-subset cuts a smaller dataset out of SIFT1M and computes its
// ground truth by brute force, for iterating against while building the index.
//
// The ground truth has to be recomputed rather than sliced out of SIFT1M's own,
// whose neighbour IDs point into the full million. See decisions.md.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/index"
)

func main() {
	var (
		src     = flag.String("src", "/mnt/d/data/sift", "directory holding sift_base.fvecs and sift_query.fvecs")
		dst     = flag.String("dst", "/mnt/d/data/sift100k", "where to write the subset")
		gt      = flag.String("gt", "bench/data/sift100k_groundtruth.ivecs", "where to write ground truth")
		nBase   = flag.Int("base", 100_000, "base vectors to keep")
		nQuery  = flag.Int("queries", 1_000, "query vectors to keep")
		k       = flag.Int("k", 100, "neighbours per query to record")
		workers = flag.Int("workers", runtime.NumCPU(), "goroutines for the brute force pass")
	)
	flag.Parse()

	if err := run(*src, *dst, *gt, *nBase, *nQuery, *k, *workers); err != nil {
		fmt.Fprintln(os.Stderr, "fastvec-subset:", err)
		os.Exit(1)
	}
}

func run(src, dst, gtPath string, nBase, nQuery, k, workers int) error {
	base, err := fvecs.ReadFloat(filepath.Join(src, "sift_base.fvecs"))
	if err != nil {
		return err
	}
	queries, err := fvecs.ReadFloat(filepath.Join(src, "sift_query.fvecs"))
	if err != nil {
		return err
	}
	if base.Len() < nBase || queries.Len() < nQuery {
		return fmt.Errorf("source has %d base and %d queries, need %d and %d",
			base.Len(), queries.Len(), nBase, nQuery)
	}

	sub := &fvecs.Float{Dim: base.Dim, Data: base.Data[:nBase*base.Dim]}
	subQ := &fvecs.Float{Dim: queries.Dim, Data: queries.Data[:nQuery*queries.Dim]}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if err := writeFvecs(filepath.Join(dst, "sift100k_base.fvecs"), sub); err != nil {
		return err
	}
	if err := writeFvecs(filepath.Join(dst, "sift100k_query.fvecs"), subQ); err != nil {
		return err
	}

	fmt.Printf("computing top-%d for %d queries over %d vectors on %d workers\n",
		k, nQuery, nBase, workers)

	f := index.NewFlat(sub)
	ids := make([]int32, nQuery*k)
	start := time.Now()

	for q := 0; q < nQuery; q++ {
		res, err := f.SearchN(subQ.At(q), k, workers)
		if err != nil {
			return fmt.Errorf("query %d: %w", q, err)
		}
		for i, r := range res {
			ids[q*k+i] = r.ID
		}
		if q > 0 && q%200 == 0 {
			fmt.Printf("  %d/%d\n", q, nQuery)
		}
	}
	fmt.Printf("done in %v\n", time.Since(start).Round(time.Millisecond))

	if err := os.MkdirAll(filepath.Dir(gtPath), 0o755); err != nil {
		return err
	}
	if err := writeIvecs(gtPath, k, ids); err != nil {
		return err
	}

	fmt.Printf("wrote %s and %s\n", dst, gtPath)
	return nil
}

func writeFvecs(path string, set *fvecs.Float) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 4+set.Dim*4)
	binary.LittleEndian.PutUint32(buf, uint32(set.Dim))

	for i := 0; i < set.Len(); i++ {
		v := set.At(i)
		for j, x := range v {
			binary.LittleEndian.PutUint32(buf[4+j*4:], math.Float32bits(x))
		}
		if _, err := f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func writeIvecs(path string, dim int, ids []int32) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 4+dim*4)
	binary.LittleEndian.PutUint32(buf, uint32(dim))

	for i := 0; i < len(ids)/dim; i++ {
		for j, id := range ids[i*dim : (i+1)*dim] {
			binary.LittleEndian.PutUint32(buf[4+j*4:], uint32(id))
		}
		if _, err := f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}
