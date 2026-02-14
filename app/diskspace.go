package app

import (
	"errors"
	"fmt"
	"math"
	"syscall"
)

var errNotEnoughDiskSpace = errors.New("not enough disk space")

// availableDiskSpace returns available bytes on the filesystem containing path.
func availableDiskSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("failed to check disk space: %w", err)
	}

	if stat.Bsize <= 0 {
		return 0, fmt.Errorf("invalid block size: %d", stat.Bsize)
	}

	available := stat.Bavail * uint64(stat.Bsize)
	if available > math.MaxInt64 {
		return math.MaxInt64, nil
	}

	return int64(available), nil
}

// checkDiskSpace verifies that there's enough space for the given size.
func checkDiskSpace(path string, needed int64) error {
	available, err := availableDiskSpace(path)
	if err != nil {
		return err
	}

	if available < needed {
		return fmt.Errorf(
			"%w: need %s, available %s",
			errNotEnoughDiskSpace,
			formatBytes(needed),
			formatBytes(available),
		)
	}

	return nil
}
