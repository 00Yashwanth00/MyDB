package storage

import (
	"os"
	"testing"
)

// TestMMap verifies that temporary files can be successfully created, sized,
// memory-mapped, mutated in memory, and cleanly unmapped without raising system errors.
func TestMMap(t *testing.T) {
	// Create a unique temporary file in the OS temp directory with "mydb-mmap-" prefix.
	file, err := os.CreateTemp("", "mydb-mmap-*")
	if err != nil {
		t.Fatal(err)
	}

	// Ensure cleanup: remove file from disk and close file descriptor when test exits.
	defer os.Remove(file.Name())
	defer file.Close()

	const size = 4096 // 4KB page size

	// Resize (extend) the empty file to 4096 bytes on disk.
	// Files must be allocated to the target size before mmapping to avoid bus errors / SIGBUS crashes.
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}

	// Map the 4096-byte file region into process memory.
	m, err := mmapFile(file, size)
	if err != nil {
		t.Fatal(err)
	}

	// Write directly to virtual memory (this mutates the underlying file region).
	m.data[0] = 42

	// Verify the byte read back from memory reflects the written value.
	if m.data[0] != 42 {
		t.Fatalf("unexpected mapped value: %d", m.data[0])
	}

	// Cleanly unmap memory space.
	if err := m.close(); err != nil {
		t.Fatal(err)
	}
}
