package index

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

// BenchmarkHNSWSearch profiles queries against an existing index. Loading and
// warming are outside benchmark timing; CPU profiles can still include setup.
// FASTVEC_BENCH_INDEX and FASTVEC_BENCH_QUERIES opt into this dataset benchmark.
func BenchmarkHNSWSearch(b *testing.B) {
	indexPath, queryPath := os.Getenv("FASTVEC_BENCH_INDEX"), os.Getenv("FASTVEC_BENCH_QUERIES")
	if indexPath == "" || queryPath == "" {
		b.Skip("set FASTVEC_BENCH_INDEX and FASTVEC_BENCH_QUERIES")
	}
	runtime.GC()
	var before, loaded runtime.MemStats
	runtime.ReadMemStats(&before)
	f, err := os.Open(indexPath)
	if err != nil {
		b.Fatal(err)
	}
	h, err := LoadHNSW(f)
	f.Close()
	if err != nil {
		b.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&loaded)
	b.Logf("retained index heap: %d bytes, %d objects", int64(loaded.HeapAlloc)-int64(before.HeapAlloc), int64(loaded.HeapObjects)-int64(before.HeapObjects))
	queries, err := fvecs.ReadFloat(queryPath)
	if err != nil {
		b.Fatal(err)
	}
	if queries.Len() == 0 || queries.Dim != h.dim {
		b.Fatal("queries must be nonempty and match index dimensions")
	}
	for _, ef := range []int{100, 500} {
		b.Run(fmt.Sprintf("ef%d", ef), func(b *testing.B) {
			h.SetEfSearch(ef)
			for i := 0; i < queries.Len(); i++ {
				if _, err := h.Search(queries.At(i), 10); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := h.Search(queries.At(i%queries.Len()), 10); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
