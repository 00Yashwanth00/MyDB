package storage

import (
	"os"
	"syscall"
)

// MMap wraps a memory-mapped byte slice along with its underlying OS file descriptor.
// Memory mapping allows direct file access via virtual memory pointers rather than read/write syscalls.
type MMap struct {
	data []byte   // The memory slice backed directly by the mapped file payload.
	file *os.File // Pointer to the open file descriptor.
}

// mmapFile maps a specified portion of an open file into the process's virtual memory space.
// Parameters:
//   - file: An open *os.File descriptor to map.
//   - size: The size in bytes to allocate for the memory region.
func mmapFile(file *os.File, size int) (*MMap, error) {
	// Execute the mmap system call to map the file into virtual memory.
	data, err := syscall.Mmap(
		int(file.Fd()),                       // Raw file descriptor integer.
		0,                                    // Offset inside the file (0 starts at beginning).
		size,                                 // Number of bytes to map into memory.
		syscall.PROT_READ|syscall.PROT_WRITE, // Pages can be read and written to.
		syscall.MAP_SHARED,                   // Share updates with other processes mapping the file and flush writes to disk.
	)

	if err != nil {
		return nil, err
	}

	return &MMap{
		data: data,
		file: file,
	}, nil
}

// close unmaps the memory region using munmap system call.
// Note: This releases the virtual memory region but does not close the underlying file descriptor.
func (m *MMap) close() error {
	return syscall.Munmap(m.data)
}
