package app

import (
	"fmt"
	"os"
)

// getResumeOffset returns the size of the .part file, or 0 if it doesn't exist.
func getResumeOffset(partPath string) int64 {
	info, err := os.Stat(partPath)
	if err != nil {
		return 0
	}
	return info.Size()
}

// finalizePart renames .part file to final path.
func finalizePart(partPath, finalPath string) error {
	if err := os.Rename(partPath, finalPath); err != nil {
		return fmt.Errorf("failed to finalize download: %w", err)
	}
	return nil
}
