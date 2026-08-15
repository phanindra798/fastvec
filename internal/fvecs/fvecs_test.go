package fvecs

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFvecs builds a .fvecs file by hand so the tests do not need the real
// datasets to run.
func writeFvecs(t *testing.T, vectors [][]float32) string {
	t.Helper()

	var buf bytes.Buffer
	for _, v := range vectors {
		binary.Write(&buf, binary.LittleEndian, int32(len(v)))
		for _, x := range v {
			binary.Write(&buf, binary.LittleEndian, math.Float32bits(x))
		}
	}

	path := filepath.Join(t.TempDir(), "test.fvecs")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadFloat(t *testing.T) {
	want := [][]float32{
		{1, 2, 3},
		{-4.5, 0, 7.25},
		{0, 0, 0},
	}
	path := writeFvecs(t, want)

	set, err := ReadFloat(path)
	if err != nil {
		t.Fatalf("ReadFloat: %v", err)
	}
	if set.Dim != 3 {
		t.Errorf("Dim = %d, want 3", set.Dim)
	}
	if set.Len() != len(want) {
		t.Fatalf("Len = %d, want %d", set.Len(), len(want))
	}
	for i, w := range want {
		got := set.At(i)
		for j := range w {
			if got[j] != w[j] {
				t.Errorf("vector %d element %d = %v, want %v", i, j, got[j], w[j])
			}
		}
	}
}

// At has to be a view, not a copy. Copying every vector during a scan would
// allocate a million times per query.
func TestAtIsAView(t *testing.T) {
	set, err := ReadFloat(writeFvecs(t, [][]float32{{1, 2}, {3, 4}}))
	if err != nil {
		t.Fatal(err)
	}

	set.At(1)[0] = 99
	if set.Data[2] != 99 {
		t.Errorf("At returned a copy, Data[2] = %v", set.Data[2])
	}
}

// Caught this by accident. Without the capacity clamp in At, appending to one
// vector writes straight over the start of the next one.
func TestAtCannotOverwriteNeighbour(t *testing.T) {
	set, err := ReadFloat(writeFvecs(t, [][]float32{{1, 2}, {3, 4}}))
	if err != nil {
		t.Fatal(err)
	}

	_ = append(set.At(0), 42)
	if got := set.At(1)[0]; got != 3 {
		t.Errorf("append to vector 0 corrupted vector 1: got %v, want 3", got)
	}
}

func TestReadFloatRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()

	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// A valid two vector file, then chopped short mid payload.
	good, err := os.ReadFile(writeFvecs(t, [][]float32{{1, 2}, {3, 4}}))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty", write("empty.fvecs", nil), "empty"},
		{"truncated", write("short.fvecs", good[:len(good)-3]), "truncated"},
		{"absurd dimension", write("huge.fvecs", []byte{0xff, 0xff, 0xff, 0x7f}), "out of range"},
		{"zero dimension", write("zero.fvecs", []byte{0, 0, 0, 0}), "out of range"},
		{"missing", filepath.Join(dir, "nope.fvecs"), "no such file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadFloat(tt.path)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

func TestReadInt(t *testing.T) {
	var buf bytes.Buffer
	rows := [][]int32{{5, 9, 1}, {0, -2, 7}}
	for _, r := range rows {
		binary.Write(&buf, binary.LittleEndian, int32(len(r)))
		binary.Write(&buf, binary.LittleEndian, r)
	}

	path := filepath.Join(t.TempDir(), "gt.ivecs")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	set, err := ReadInt(path)
	if err != nil {
		t.Fatalf("ReadInt: %v", err)
	}
	if set.Len() != 2 || set.Dim != 3 {
		t.Fatalf("got %d rows of %d, want 2 of 3", set.Len(), set.Dim)
	}
	for i, r := range rows {
		got := set.At(i)
		for j := range r {
			if got[j] != r[j] {
				t.Errorf("row %d element %d = %d, want %d", i, j, got[j], r[j])
			}
		}
	}
}
