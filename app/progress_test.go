package app

import (
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProgressReader(t *testing.T) {
	data := strings.Repeat("x", 1000)
	r := strings.NewReader(data)

	var currentRead int64
	pr := &progressReader{
		reader:     r,
		total:      1000,
		onProgress: func(n int64) { currentRead = n },
	}

	buf := make([]byte, 100)
	totalRead := 0
	for {
		n, err := pr.Read(buf)
		totalRead += n
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	if totalRead != 1000 {
		t.Errorf("total read = %d, want 1000", totalRead)
	}
	if currentRead != 1000 {
		t.Errorf("progress reported = %d, want 1000", currentRead)
	}
}

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
