package storage

import (
	"fmt"
	"os"
)

// SaveData1 updating the file in-plac by overwriting it[cite: 1].
// This method directly opens and writes to the target path, which is straightforward but can cause data corruption if the system crashes mid-write.
func SaveData1(path string, data []byte) error {
	// os.OpenFile opens the file using the following flags[cite: 1]:
	// os.O_WRONLY: Open for write-only access[cite: 1].
	// os.O_CREATE: Create the file if it does not already exist[cite: 1].
	// os.O_TRUNC: Truncate the file to 0 length if it exists, overwriting all old data[cite: 1].
	// 0664 specifies the Unix permissions (read/write for owner and group, read for others)[cite: 1].
	fp, err := os.OpenFile(
		path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0664,
	)

	// Immediately return any error encountered while attempting to open the file[cite: 1].
	if err != nil {
		return err
	}

	// defer ensures the file descriptor is safely closed when the function exits, preventing resource leaks[cite: 1].
	defer fp.Close()

	// Write the byte slice data directly to the file[cite: 1].
	_, err = fp.Write(data)
	if err != nil {
		return err
	}

	// fp.Sync() flushes the operating system's memory buffers down to the physical disk to guarantee the data is safely persisted[cite: 1].
	return fp.Sync()
}

// SaveData2 renaming the file by creating a new file[cite: 1].
// This implements a safer, atomic-write pattern that prevents partial or corrupted writes.
func SaveData2(path string, data []byte) error {
	// Generate a unique temporary file name by appending ".tmp." and the current OS Process ID (os.Getpid()) to the target path[cite: 1].
	tmp := fmt.Sprintf("%s.tmp.%d", path, os.Getpid())

	// Open the temporary file with the os.O_EXCL flag[cite: 1].
	// os.O_EXCL ensures that this call exclusively creates the file; if the temporary file already exists, it will safely fail[cite: 1].
	fp, err := os.OpenFile(
		tmp,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0664,
	)
	if err != nil {
		return err
	}

	// Defer an anonymous cleanup function that will execute right before SaveData2 returns[cite: 1].
	defer func() {
		// Close the temporary file descriptor[cite: 1].
		fp.Close()

		// If an error occurred at any point during writing or syncing, delete (os.Remove) the temporary file to avoid leaving orphaned garbage files on the disk[cite: 1].
		if err != nil {
			os.Remove(tmp)
		}
	}()

	// Write the payload data to the temporary file[cite: 1].
	_, err = fp.Write(data)
	if err != nil {
		return err
	}

	// Flush the written data to the physical disk to ensure the temporary file is complete before we attempt to rename it[cite: 1].
	err = fp.Sync()
	if err != nil {
		return err
	}

	// os.Rename changes the temporary file to the final target path[cite: 1].
	// On POSIX systems, this rename operation replaces the destination file atomically, ensuring no concurrent reader ever encounters a partially written file.
	return os.Rename(tmp, path)
}

// syncDir flushes directory metadata changes (such as file creations,
// deletions, or renames) to disk to ensure durability.
func syncDir(path string) error {
	// Open a file handle for the directory path.
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	// Ensure the directory file descriptor is closed when the function exits.
	defer dir.Close()

	// Issue an fsync system call on the directory descriptor to flush
	// any pending directory entry modifications to durable storage.
	return dir.Sync()
}
