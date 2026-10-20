package main

import (
	"fmt"
	"time"
)

// Repeat whole query sweeps until the measured interval is meaningful. Earlier
// calibration attempts are excluded. A broken/stalled clock fails explicitly.
func measureThroughput(sweep func() error, queries int, now func() time.Time) (int64, time.Duration, error) {
	if queries < 1 {
		return 0, 0, fmt.Errorf("throughput measurement needs at least one query")
	}
	// Tiny datasets can finish a million sweeps in under 100 ms. Allow
	// additional calibration while retaining a finite stalled-clock limit.
	for repeats := int64(1); repeats <= 1<<24; repeats *= 2 {
		if int64(queries) > (1<<63-1)/repeats {
			return 0, 0, fmt.Errorf("timed query count exceeds int64 limits")
		}
		start := now()
		for i := int64(0); i < repeats; i++ {
			if err := sweep(); err != nil {
				return 0, 0, err
			}
		}
		elapsed := now().Sub(start)
		if elapsed < 0 {
			return 0, 0, fmt.Errorf("benchmark clock moved backwards")
		}
		if elapsed >= 100*time.Millisecond {
			return repeats * int64(queries), elapsed, nil
		}
	}
	return 0, 0, fmt.Errorf("benchmark clock resolution is insufficient after repeated sweeps")
}
