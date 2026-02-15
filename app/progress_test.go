package app

import (
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFormatProgressBar(t *testing.T) {
	bar := formatProgressBar(50, 100)
	if !strings.Contains(bar, "50%") {
		t.Errorf("bar should contain 50%%, got %q", bar)
	}

	bar = formatProgressBar(100, 100)
	if !strings.Contains(bar, "100%") {
		t.Errorf("bar should contain 100%%, got %q", bar)
	}

	bar = formatProgressBar(0, 100)
	if !strings.Contains(bar, " 0%") {
		t.Errorf("bar should contain 0%%, got %q", bar)
	}
}

func TestFormatProgressBarUnknownTotal(t *testing.T) {
	bar := formatProgressBar(500, 0)
	if !strings.Contains(bar, "?") {
		t.Errorf("bar should contain '?' for unknown total, got %q", bar)
	}
	if !strings.Contains(bar, "500B") {
		t.Errorf("bar should contain byte count, got %q", bar)
	}

	bar2 := formatProgressBar(2048, -1)
	if !strings.Contains(bar2, "?") {
		t.Errorf("bar should contain '?' for negative total, got %q", bar2)
	}
	if !strings.Contains(bar2, "2.0KB") {
		t.Errorf("bar should contain formatted byte count, got %q", bar2)
	}
}

func TestFormatProgressBarOverflow(t *testing.T) {
	bar := formatProgressBar(200, 100)
	if !strings.Contains(bar, "100%") {
		t.Errorf("bar should cap at 100%% when current > total, got %q", bar)
	}
}

func TestTruncateFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		want     string
	}{
		{"short enough", "file.zip", 20, "file.zip"},
		{"exact fit", "file.zip", 8, "file.zip"},
		{"needs truncation", "very-long-filename.zip", 12, "very-long..."},
		{"minimal width", "abcdef", 3, "abc"},
		{"ellipsis boundary", "abcdef", 4, "a..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateFilename(tt.input, tt.maxWidth)
			if got != tt.want {
				t.Errorf("truncateFilename(%q, %d) = %q, want %q", tt.input, tt.maxWidth, got, tt.want)
			}
		})
	}
}

func TestCountingReader(t *testing.T) {
	data := strings.Repeat("x", 500)
	r := strings.NewReader(data)

	var counter atomic.Int64
	cr := &countingReader{reader: r, counter: &counter}

	buf := make([]byte, 100)
	totalRead := 0

	for {
		n, err := cr.Read(buf)
		totalRead += n

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatal(err)
		}
	}

	if totalRead != 500 {
		t.Errorf("total read = %d, want 500", totalRead)
	}

	if counter.Load() != 500 {
		t.Errorf("counter = %d, want 500", counter.Load())
	}
}

func TestProgressStartFinishRender(t *testing.T) {
	pd := newProgressDisplay()

	var counter atomic.Int64
	counter.Store(50)
	pd.add("test-file.bin", &counter, 100)

	pd.start()
	time.Sleep(500 * time.Millisecond) // allow at least 1 render tick
	pd.finish()
}

func TestProgressDisplayMultipleEntries(t *testing.T) {
	pd := newProgressDisplay()

	var c1, c2 atomic.Int64
	c1.Store(30)
	c2.Store(0)
	pd.add("file1.bin", &c1, 100)
	pd.add("file2.bin", &c2, 200)

	pd.start()
	c2.Store(100)
	time.Sleep(500 * time.Millisecond)
	pd.finish()
}
