package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"valid http", "http://example.com/file.zip", "http://example.com/file.zip", false},
		{"valid https", "https://example.com/file.zip", "https://example.com/file.zip", false},
		{"ftp scheme", "ftp://example.com/file.zip", "", true},
		{"no scheme", "example.com/file.zip", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseValidateURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseValidateURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseValidateURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindExtension(t *testing.T) {
	tests := []struct {
		mimeType string
		want     string
	}{
		{"image/jpeg", "jpg"},
		{"video/mp4", "mp4"},
		{"totally/bogus-not-real", "unknown"},
		{"text/html", "ehtml"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := findExtension(tt.mimeType)
			if got != tt.want {
				t.Errorf("findExtension(%q) = %q, want %q", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestGetChunks(t *testing.T) {
	chunks := getChunks(100, 5)
	if len(chunks) != 5 {
		t.Fatalf("expected 5 chunks, got %d", len(chunks))
	}
	if chunks[0][0] != 0 {
		t.Errorf("first chunk start = %d, want 0", chunks[0][0])
	}
	if chunks[len(chunks)-1][1] != 99 {
		t.Errorf("last chunk end = %d, want 99", chunks[len(chunks)-1][1])
	}

	// verify no gaps or overlaps
	var total int64
	for i, c := range chunks {
		size := c[1] - c[0] + 1
		total += size
		if i > 0 && c[0] != chunks[i-1][1]+1 {
			t.Errorf("gap between chunk %d and %d", i-1, i)
		}
	}
	if total != 100 {
		t.Errorf("total bytes covered = %d, want 100", total)
	}
}

func TestGetChunksSmallFile(t *testing.T) {
	// file smaller than chunk count — should cap chunks
	chunks := getChunks(3, 5)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0] != [2]int64{0, 0} {
		t.Errorf("chunk[0] = %v, want [0, 0]", chunks[0])
	}
	if chunks[2] != [2]int64{2, 2} {
		t.Errorf("chunk[2] = %v, want [2, 2]", chunks[2])
	}
}

func TestGetChunksSingleByte(t *testing.T) {
	chunks := getChunks(1, 5)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != [2]int64{0, 0} {
		t.Errorf("chunk[0] = %v, want [0, 0]", chunks[0])
	}
}

func TestGetChunksZero(t *testing.T) {
	if chunks := getChunks(0, 5); chunks != nil {
		t.Errorf("expected nil for zero length, got %v", chunks)
	}
}

func TestParseRate(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"5M", 5 * 1024 * 1024, false},
		{"500K", 500 * 1024, false},
		{"1G", 1024 * 1024 * 1024, false},
		{"1.5M", int64(1.5 * 1024 * 1024), false},
		{"abc", 0, true},
		{"-5M", 0, true},
		{"-100K", 0, true},
		{"9999999999G", 0, true},
		{"NaN", 0, true},
		{"Inf", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseRate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseRate(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestDeduplicateFilenames(t *testing.T) {
	resources := []*resource{
		{filename: "file.zip"},
		{filename: "file.zip"},
		{filename: "other.tar.gz"},
		{filename: "file.zip"},
	}

	deduplicateFilenames(resources, "")

	want := []string{"file.zip", "file_1.zip", "other.tar.gz", "file_2.zip"}
	for i, r := range resources {
		if r.filename != want[i] {
			t.Errorf("resources[%d].filename = %q, want %q", i, r.filename, want[i])
		}
	}
}

func TestDeduplicateFilenamesWithExistingSuffix(t *testing.T) {
	resources := []*resource{
		{filename: "file.zip"},
		{filename: "file_1.zip"},
		{filename: "file.zip"},
	}

	deduplicateFilenames(resources, "")

	want := []string{"file.zip", "file_1.zip", "file_2.zip"}
	for i, r := range resources {
		if r.filename != want[i] {
			t.Errorf("resources[%d].filename = %q, want %q", i, r.filename, want[i])
		}
	}
}

func TestDeduplicateFilenamesNoDuplicates(t *testing.T) {
	resources := []*resource{
		{filename: "a.zip"},
		{filename: "b.zip"},
		{filename: "c.zip"},
	}

	deduplicateFilenames(resources, "")

	want := []string{"a.zip", "b.zip", "c.zip"}
	for i, r := range resources {
		if r.filename != want[i] {
			t.Errorf("resources[%d].filename = %q, want %q", i, r.filename, want[i])
		}
	}
}

func TestDeduplicateFilenamesExistingOnDisk(t *testing.T) {
	dir := t.TempDir()

	// create existing file on disk
	if err := os.WriteFile(filepath.Join(dir, "file.zip"), []byte("x"), permFile); err != nil {
		t.Fatal(err)
	}

	resources := []*resource{
		{filename: "file.zip"},
		{filename: "other.zip"},
	}

	deduplicateFilenames(resources, dir)

	if resources[0].filename != "file_1.zip" {
		t.Errorf("resources[0].filename = %q, want 'file_1.zip' (should avoid disk collision)", resources[0].filename)
	}
	if resources[1].filename != "other.zip" {
		t.Errorf("resources[1].filename = %q, want 'other.zip'", resources[1].filename)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{500, "500B"},
		{1024, "1.0KB"},
		{1024 * 1024, "1.0MB"},
		{1024 * 1024 * 1024, "1.0GB"},
		{1536 * 1024, "1.5MB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBytes(tt.input)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
