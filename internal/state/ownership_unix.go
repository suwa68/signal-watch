//go:build unix

package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// The file is intentionally never unlinked: all owners must lock the same inode,
// including owners using symlink/relative aliases of the directory.
func acquireOwnership(directory string) (*os.File, error) {
	file, err := os.OpenFile(filepath.Join(directory, ".signalwatch.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, errors.Join(fmt.Errorf("acquire state directory ownership: %w", err), file.Close())
	}
	return file, nil
}
