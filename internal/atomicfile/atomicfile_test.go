package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.bin")

	for _, want := range []string{"first", "second, longer than the first"} {
		err := Write(path, func(w io.Writer) error {
			_, err := io.WriteString(w, want)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	assertOnlyFile(t, path)
}

// A failure partway through must leave the previous file exactly as it was
// and no temporary file behind. This is the truncated-cache case.
func TestWriteFailureKeepsOldFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.fvi")
	if err := os.WriteFile(path, []byte("complete old index"), 0o644); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("disk full")
	err := Write(path, func(w io.Writer) error {
		if _, err := io.WriteString(w, "half of a new"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "complete old index" {
		t.Fatalf("old file changed to %q", got)
	}
	assertOnlyFile(t, path)
}

func TestWriteFailureLeavesNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.fvi")
	err := Write(path, func(io.Writer) error { return errors.New("build failed") })
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target exists after failed write: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("left %d files behind", len(entries))
	}
}

func assertOnlyFile(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory holds %v, want only %s", names, filepath.Base(path))
	}
}
