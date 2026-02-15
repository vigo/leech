package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetResumeOffset(t *testing.T) {
	dir := t.TempDir()
	partPath := filepath.Join(dir, "test.zip.part")

	offset := getResumeOffset(partPath)
	if offset != 0 {
		t.Errorf("expected 0 offset for missing file, got %d", offset)
	}

	if err := os.WriteFile(partPath, make([]byte, 500), 0o644); err != nil {
		t.Fatal(err)
	}

	offset = getResumeOffset(partPath)
	if offset != 500 {
		t.Errorf("expected 500 offset, got %d", offset)
	}
}

func TestFinalizePart(t *testing.T) {
	dir := t.TempDir()
	partPath := filepath.Join(dir, "test.zip.part")
	finalPath := filepath.Join(dir, "test.zip")

	if err := os.WriteFile(partPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := finalizePart(partPath, finalPath); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Error("part file should be removed after finalize")
	}
	if _, err := os.Stat(finalPath); err != nil {
		t.Error("final file should exist after finalize")
	}
}
