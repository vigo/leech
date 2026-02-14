//go:build unix

package app

import "testing"

func TestAvailableDiskSpaceInvalidPath(t *testing.T) {
	_, err := availableDiskSpace("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
