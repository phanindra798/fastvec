// Package atomicfile writes a file so that it either appears complete or not
// at all.
//
// The benchmark caches a SIFT1M index as a 550 MB file. Written in place, a
// failed or interrupted save left a truncated file at the cache path, and the
// next run refused to load it instead of rebuilding. Writing to a temporary file
// in the same directory and renaming it over the target means a reader only
// ever sees the old file, no file, or the finished one.
package atomicfile

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
)

// Write calls fill with a buffered writer, then flushes, syncs and closes the
// temporary file and renames it to path. Any error removes the temporary file
// and leaves whatever was at path untouched.
func Write(path string, fill func(io.Writer) error) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	bw := bufio.NewWriterSize(tmp, 1<<20)
	if err = fill(bw); err != nil {
		return err
	}
	if err = bw.Flush(); err != nil {
		return err
	}
	// Close can report a failed write the earlier calls did not, so its error
	// matters as much as theirs. Sync first so a crash after the rename cannot
	// expose a file whose data never reached the disk.
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file 0600. Match what os.Create would have given.
	if err = os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
