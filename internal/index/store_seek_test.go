package index

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"strings"
	"testing"
)

type changingGraphReader struct {
	*bytes.Reader
	data                    []byte
	failCurrent, failRewind bool
	mutate                  bool
	seekCalls               int
}

func (r *changingGraphReader) Seek(offset int64, whence int) (int64, error) {
	r.seekCalls++
	if (whence == io.SeekCurrent && r.failCurrent) || (whence == io.SeekStart && r.failRewind) {
		return 0, errors.New("seek unavailable")
	}
	if whence == io.SeekStart && r.mutate {
		// Drop a bottom-layer edge between passes, keeping the count valid.
		r.data[offset]--
	}
	return r.Reader.Seek(offset, whence)
}

func TestLoadSeekAndStreamPaths(t *testing.T) {
	h, err := BuildHNSW(randomSet(rand.New(rand.NewSource(33)), 100, 8), Params{M: 8, EfConstruct: 40})
	if err != nil {
		t.Fatal(err)
	}
	var saved bytes.Buffer
	if err := h.Save(&saved); err != nil {
		t.Fatal(err)
	}
	data := saved.Bytes()
	t.Run("explicit single pass never seeks", func(t *testing.T) {
		r := &changingGraphReader{Reader: bytes.NewReader(data), failCurrent: true, failRewind: true}
		loaded, err := LoadHNSWWithOptions(r, LoadOptions{SinglePass: true})
		if err != nil {
			t.Fatal(err)
		}
		if r.seekCalls != 0 {
			t.Fatalf("single pass attempted %d seeks", r.seekCalls)
		}
		var roundtrip bytes.Buffer
		if err := loaded.Save(&roundtrip); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, roundtrip.Bytes()) {
			t.Fatal("single pass changed graph")
		}
		if _, err := LoadHNSWWithOptions(bytes.NewReader(data[:len(data)-1]), LoadOptions{SinglePass: true}); err == nil {
			t.Fatal("single pass accepted truncated data")
		}
	})
	prefixed := bytes.NewReader(append(make([]byte, 37), data...))
	if _, err := prefixed.Seek(37, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	for name, reader := range map[string]io.Reader{
		"seekable":                     bytes.NewReader(data),
		"stream":                       struct{ io.Reader }{bytes.NewReader(data)},
		"embedded":                     prefixed,
		"unavailable current position": &changingGraphReader{Reader: bytes.NewReader(data), failCurrent: true},
	} {
		t.Run(name, func(t *testing.T) {
			loaded, err := LoadHNSW(reader)
			if err != nil {
				t.Fatal(err)
			}
			var roundtrip bytes.Buffer
			if err := loaded.Save(&roundtrip); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, roundtrip.Bytes()) {
				t.Fatal("serialized graph changed")
			}
		})
	}
	for _, scenario := range []struct {
		name, want   string
		fail, mutate bool
	}{
		{"rewind failure", "rewinding graph", true, false},
		{"changed count", "changed between graph passes", false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			copyData := append([]byte(nil), data...)
			r := &changingGraphReader{Reader: bytes.NewReader(copyData), data: copyData, failRewind: scenario.fail, mutate: scenario.mutate}
			if _, err := LoadHNSW(r); err == nil || !strings.Contains(err.Error(), scenario.want) {
				t.Fatalf("got %v, want %s", err, scenario.want)
			}
		})
	}
}
