package app

import (
	"errors"
	"testing"
)

func TestAvailableDiskSpace(t *testing.T) {
	space, err := availableDiskSpace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if space <= 0 {
		t.Errorf("expected positive disk space, got %d", space)
	}
}

func TestCheckDiskSpace(t *testing.T) {
	// should pass — requesting 1 byte on temp dir
	err := checkDiskSpace(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckDiskSpaceInsufficient(t *testing.T) {
	// request absurdly large amount
	err := checkDiskSpace(t.TempDir(), 1<<62)
	if err == nil {
		t.Error("expected error for insufficient disk space")
	}
	if !errors.Is(err, errNotEnoughDiskSpace) {
		t.Errorf("expected errNotEnoughDiskSpace, got %v", err)
	}
}

func TestCheckDiskSpaceInvalidPath(t *testing.T) {
	err := checkDiskSpace("/nonexistent/path/that/does/not/exist", 1)
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
