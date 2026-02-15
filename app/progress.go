package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const progressBarWidth = 30

type countingReader struct {
	reader  io.Reader
	counter *atomic.Int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.reader.Read(p)
	if n > 0 {
		cr.counter.Add(int64(n))
	}

	return n, err
}

// progressDisplay manages multi-line progress output for concurrent downloads.
type progressDisplay struct {
	mu      sync.Mutex
	entries []progressEntry
	lines   int
	stop    chan struct{}
	done    chan struct{}
}

type progressEntry struct {
	filename string
	current  *atomic.Int64
	total    int64
}

func newProgressDisplay() *progressDisplay {
	return &progressDisplay{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

func (pd *progressDisplay) add(filename string, current *atomic.Int64, total int64) {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	pd.entries = append(pd.entries, progressEntry{
		filename: filename,
		current:  current,
		total:    total,
	})
}

func (pd *progressDisplay) start() {
	go func() {
		defer close(pd.done)

		ticker := time.NewTicker(progressUpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				pd.render()
			case <-pd.stop:
				pd.render()

				return
			}
		}
	}()
}

func (pd *progressDisplay) finish() {
	close(pd.stop)
	<-pd.done

	fmt.Fprint(os.Stderr, "\n")
}

func (pd *progressDisplay) render() {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	// move cursor up to overwrite previous output
	if pd.lines > 0 {
		fmt.Fprintf(os.Stderr, "\033[%dA", pd.lines)
	}

	termWidth := terminalWidth()

	// pre-compute bars and find max lengths
	bars := make([]string, len(pd.entries))
	maxBarLen := 0
	maxNameLen := 0

	for i, e := range pd.entries {
		bars[i] = formatProgressBar(e.current.Load(), e.total)
		if len(bars[i]) > maxBarLen {
			maxBarLen = len(bars[i])
		}
		if len(e.filename) > maxNameLen {
			maxNameLen = len(e.filename)
		}
	}

	// ": " separator = 2 chars
	const separatorLen = 2
	const minNameWidth = 10

	availableForName := termWidth - maxBarLen - separatorLen
	if availableForName < minNameWidth {
		availableForName = minNameWidth
	}
	if maxNameLen > availableForName {
		maxNameLen = availableForName
	}

	for i, e := range pd.entries {
		name := truncateFilename(e.filename, maxNameLen)
		fmt.Fprintf(os.Stderr, "\r\033[K%*s: %s\n", maxNameLen, name, bars[i])
	}

	pd.lines = len(pd.entries)
}

func truncateFilename(name string, maxWidth int) string {
	if len(name) <= maxWidth {
		return name
	}

	const ellipsis = "..."
	if maxWidth <= len(ellipsis) {
		return name[:maxWidth]
	}

	return name[:maxWidth-len(ellipsis)] + ellipsis
}

// formatProgressBar renders: [████████░░░░░░░] 50% 5.0MB/10.0MB
func formatProgressBar(current, total int64) string {
	if total <= 0 {
		return fmt.Sprintf("[%s] %s", strings.Repeat("?", progressBarWidth), formatBytes(current))
	}

	pct := float64(current) / float64(total)
	if pct > 1 {
		pct = 1
	}

	filled := int(pct * float64(progressBarWidth))
	empty := progressBarWidth - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	return fmt.Sprintf("[%s] %3.0f%% %s/%s", bar, pct*100, formatBytes(current), formatBytes(total))
}
