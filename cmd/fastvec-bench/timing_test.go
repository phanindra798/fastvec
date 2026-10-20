package main

import (
	"errors"
	"testing"
	"time"
)

func TestThroughputRetriesZeroDuration(t *testing.T) {
	times := []time.Duration{0, 0, 0, 200 * time.Millisecond}
	now := func() time.Time { v := times[0]; times = times[1:]; return time.Unix(0, int64(v)) }
	calls := 0
	queries, elapsed, err := measureThroughput(func() error { calls++; return nil }, 3, now)
	if err != nil || queries != 6 || elapsed != 200*time.Millisecond || calls != 3 {
		t.Fatalf("queries=%d elapsed=%v calls=%d err=%v", queries, elapsed, calls, err)
	}
}

func TestThroughputCalibratesVeryFastSweeps(t *testing.T) {
	var calls int64
	now := func() time.Time { return time.Unix(0, calls*50) }
	queries, elapsed, err := measureThroughput(func() error { calls++; return nil }, 1, now)
	if err != nil || queries != 1<<21 || elapsed < 100*time.Millisecond {
		t.Fatalf("queries=%d elapsed=%v err=%v", queries, elapsed, err)
	}
}

func TestThroughputRejectsUnusableClock(t *testing.T) {
	for _, backwards := range []bool{false, true} {
		n := int64(0)
		now := func() time.Time {
			if backwards {
				n--
			}
			return time.Unix(n, 0)
		}
		if _, _, err := measureThroughput(func() error { return nil }, 1, now); err == nil {
			t.Fatalf("bad clock accepted: backwards=%v", backwards)
		}
	}
}

func TestThroughputPropagatesSearchFailure(t *testing.T) {
	want := errors.New("search failed")
	_, _, err := measureThroughput(func() error { return want }, 1, time.Now)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
	if _, _, err := measureThroughput(func() error { t.Fatal("empty input searched"); return nil }, 0, time.Now); err == nil {
		t.Fatal("empty input accepted")
	}
}
