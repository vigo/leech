//go:build windows

package app

import "math"

// availableDiskSpace is not implemented on Windows; skip disk space checks.
func availableDiskSpace(_ string) (int64, error) {
	return math.MaxInt64, nil
}
