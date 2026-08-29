package index

import (
	"bytes"
	"math/rand"
	"testing"
)

// A loaded index has to answer exactly what the built one did. Anything less
// and a saved index is a different index wearing the same name.
func TestSaveLoadRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(51))
	set := randomSet(r, 2000, 16)

	built, err := BuildHNSW(set, Params{M: 8, EfConstruct: 60, EfSearch: 50, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := built.Save(&buf); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d nodes, %d dims, %d bytes on disk", built.Len(), built.Dim(), buf.Len())

	loaded, err := LoadHNSW(&buf)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Len() != built.Len() || loaded.Dim() != built.Dim() {
		t.Fatalf("loaded %dx%d, built %dx%d", loaded.Len(), loaded.Dim(), built.Len(), built.Dim())
	}
	if loaded.MaxLevel() != built.MaxLevel() {
		t.Errorf("maxLevel %d, want %d", loaded.MaxLevel(), built.MaxLevel())
	}
	if loaded.EfSearch() != built.EfSearch() {
		t.Errorf("efSearch %d, want %d", loaded.EfSearch(), built.EfSearch())
	}

	bc, bl := built.Components()
	lc, ll := loaded.Components()
	if bc != lc || bl != ll {
		t.Errorf("loaded has %d components largest %d, built had %d and %d", lc, ll, bc, bl)
	}

	for q := 0; q < 300; q++ {
		query := make([]float32, 16)
		for i := range query {
			query[i] = r.Float32()
		}

		want, err := built.Search(query, 10)
		if err != nil {
			t.Fatal(err)
		}
		got, err := loaded.Search(query, 10)
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != len(want) {
			t.Fatalf("query %d: %d results, want %d", q, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("query %d result %d: %+v, want %+v", q, i, got[i], want[i])
			}
		}
	}
}

// Same seed, same input, same bytes. Without this a benchmark can't be repeated
// from a saved index and be trusted.
func TestSaveIsDeterministic(t *testing.T) {
	r := rand.New(rand.NewSource(52))
	set := randomSet(r, 800, 8)

	dump := func() []byte {
		h, err := BuildHNSW(set, Params{M: 8, EfConstruct: 40, EfSearch: 40, Seed: 99})
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err := h.Save(&b); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}

	if a, b := dump(), dump(); !bytes.Equal(a, b) {
		t.Errorf("two builds with the same seed produced different files, %d and %d bytes", len(a), len(b))
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	r := rand.New(rand.NewSource(53))
	h, err := BuildHNSW(randomSet(r, 200, 4), Params{M: 4, EfConstruct: 20, EfSearch: 20})
	if err != nil {
		t.Fatal(err)
	}
	var good bytes.Buffer
	if err := h.Save(&good); err != nil {
		t.Fatal(err)
	}

	corrupt := func(name string, b []byte) {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadHNSW(bytes.NewReader(b)); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}

	corrupt("empty", nil)
	corrupt("wrong magic", []byte("NOTVEC\x01\x00garbage padding here"))

	truncated := append([]byte(nil), good.Bytes()...)
	corrupt("truncated", truncated[:len(truncated)/2])

	bumped := append([]byte(nil), good.Bytes()...)
	bumped[len(storeMagic)] = 99 // version field
	corrupt("future version", bumped)
}
