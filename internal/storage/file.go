package storage

import (
	"fmt"
	"os"
)

// updating the file in-plac by overwriting it
func SaveData1(path string, data []byte) error {
	fp, err := os.OpenFile(
		path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0664,
	)

	if err != nil {
		return err
	}

	defer fp.Close()

	_, err = fp.Write(data)
	if err != nil {
		return err
	}

	return fp.Sync()
}

// renaming the file by creating a new file
func SaveData2(path string, data []byte) error {
	tmp := fmt.Sprintf("%s.tmp.%d", path, os.Getpid())

	fp, err := os.OpenFile(
		tmp,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0664,
	)
	if err != nil {
		return err
	}

	defer func() {
		fp.Close()

		if err != nil {
			os.Remove(tmp)
		}
	}()

	_, err = fp.Write(data)
	if err != nil {
		return err
	}

	err = fp.Sync()
	if err != nil {
		return err
	}

	return os.Rename(tmp, path)
}
