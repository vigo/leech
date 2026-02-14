package app

import (
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
	total := 0
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
	if chunks[0] != [2]int{0, 0} {
		t.Errorf("chunk[0] = %v, want [0, 0]", chunks[0])
	}
	if chunks[2] != [2]int{2, 2} {
		t.Errorf("chunk[2] = %v, want [2, 2]", chunks[2])
	}
}

func TestGetChunksSingleByte(t *testing.T) {
	chunks := getChunks(1, 5)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != [2]int{0, 0} {
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
