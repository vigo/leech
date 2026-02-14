package app

import (
	"errors"
	"fmt"
)

var errNotEnoughDiskSpace = errors.New("not enough disk space")

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
