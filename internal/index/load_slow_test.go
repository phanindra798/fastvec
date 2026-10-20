package index

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// pacedIndexReader models aggregate read throughput, including rereads after
// seeking. It is a controlled simulation, not a cold-disk benchmark.
type pacedIndexReader struct {
	*os.File
	start time.Time
	bytes int64
	rate  int64
}

func (r *pacedIndexReader) Read(p []byte) (int, error) {
	n, err := r.File.Read(p)
	r.bytes += int64(n)
	deadline := r.start.Add(time.Duration(float64(r.bytes) / float64(r.rate) * float64(time.Second)))
	if delay := time.Until(deadline); delay > 0 {
		time.Sleep(delay)
	}
	return n, err
}

func TestLoadPacedComparison(t *testing.T) {
	path := os.Getenv("FASTVEC_LOAD_SLOW")
	if testing.Short() || path == "" {
		t.Skip("set FASTVEC_LOAD_SLOW for simulated slow reads")
	}
	for pair := 1; pair <= 3; pair++ {
		modes := []bool{false, true}
		if pair == 2 {
			modes = []bool{true, false}
		}
		for _, single := range modes {
			runtime.GC()
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			reader := &pacedIndexReader{File: f, start: time.Now(), rate: 100_000_000}
			h, err := LoadHNSWWithOptions(reader, LoadOptions{SinglePass: single})
			elapsed := time.Since(reader.start)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("pair=%d single_pass=%v bytes_read=%d elapsed_ns=%d rate_bytes_sec=%d nodes=%d", pair, single, reader.bytes, elapsed.Nanoseconds(), reader.rate, h.Len())
			runtime.KeepAlive(h)
		}
	}
}
