package index

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// One load per process; an external process monitor can record peak RSS.
func TestLoadMeasurement(t *testing.T) {
	path := os.Getenv("FASTVEC_LOAD_MEASURE")
	if path == "" {
		t.Skip("set FASTVEC_LOAD_MEASURE for a standalone load measurement")
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	h, err := LoadHNSW(f)
	elapsed := time.Since(start)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	t.Logf("load_ns=%d allocated_bytes=%d allocations=%d heap_bytes=%d nodes=%d", elapsed.Nanoseconds(), after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs, after.HeapAlloc, h.Len())
	// Leave the process available briefly for the external peak-RSS monitor.
	time.Sleep(200 * time.Millisecond)
	runtime.KeepAlive(h)
}
