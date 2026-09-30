package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSyncDir verifies that syncDir successfully flushes directory metadata
// to disk after a file creation event without raising filesystem errors.
func TestSyncDir(t *testing.T) {
	// Create an isolated temporary directory managed by the Go testing framework.
	// t.TempDir automatically cleans up the directory and its contents when the test finishes.
	dir := t.TempDir()

	// Construct the path for a dummy database file inside the temp directory.
	file := filepath.Join(dir, "test.db")

	// Create the file to trigger a directory metadata change (adding a directory entry).
	fp, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}

	// Close the file descriptor before syncing the directory.
	if err := fp.Close(); err != nil {
		t.Fatal(err)
	}

	// Call syncDir on the directory containing the new file entry to verify
	// that directory flushing completes without returning an OS-level error.
	if err := syncDir(dir); err != nil {
		t.Fatal(err)
	}
}
