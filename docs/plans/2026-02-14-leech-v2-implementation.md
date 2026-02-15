# Leech v2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Modernize and complete the Leech concurrent download manager with progress bar, bandwidth limiting, resume support, structured logging, and CI/CD tooling.

**Architecture:** Split monolithic `app.go` into focused files. Each feature (progress, ratelimit, resume) is an `io.Reader` wrapper composed in the download pipeline. `log/slog` for all output. Zero external dependencies for core functionality.

**Tech Stack:** Go 1.26, log/slog, golangci-lint v2, pre-commit, GitHub Actions

---

### Task 1: Modernize Go Module and Fix Deprecated APIs

**Files:**
- Modify: `go.mod`
- Modify: `app/app.go:5-6` (imports), `app/app.go:324-326` (ioutil usage)

**Step 1: Update go.mod**

```
module github.com/vigo/leech

go 1.26
```

**Step 2: Replace deprecated ioutil calls in app.go**

Replace `"io/ioutil"` import with nothing (already has `"io"`).

In `fetch()` function, replace:
```go
b, err := ioutil.ReadAll(resp.Body)
```
with:
```go
b, err := io.ReadAll(resp.Body)
```

Remove the commented-out `ioutil.Discard` line.

**Step 3: Verify it compiles**

Run: `cd /Users/vigo/Repos/Development/vigo/golang/cli/leech && go build ./...`
Expected: SUCCESS, no errors

**Step 4: Commit**

```bash
git add go.mod app/app.go
git commit -m "modernize: update to Go 1.26, replace deprecated ioutil"
```

---

### Task 2: Extract Helpers into helpers.go with Tests

**Files:**
- Create: `app/helpers.go`
- Create: `app/helpers_test.go`
- Modify: `app/app.go` (remove extracted functions)

**Step 1: Create app/helpers.go**

Extract these functions from `app/app.go` into `app/helpers.go`:

```go
package app

import (
	"fmt"
	"mime"
	"net/url"
	"strconv"
	"strings"
)

func isPiped() bool {
	fileInfo, _ := os.Stdin.Stat()
	return fileInfo.Mode()&os.ModeCharDevice == 0
}

func parseValidateURL(in string) (string, error) {
	u, err := url.ParseRequestURI(in)
	if err != nil {
		return "", fmt.Errorf("%s %w", errInvalidURL.Error(), err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errInvalidURL
	}
	return u.String(), nil
}

func findExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return "jpg"
	case "video/mp4":
		return "mp4"
	default:
		ext, err := mime.ExtensionsByType(mimeType)
		if err != nil || len(ext) == 0 {
			return "unknown"
		}
		return ext[0]
	}
}

func getChunks(length int, chunkSize int) [][2]int {
	out := [][2]int{}
	chunk := length / chunkSize

	start := 0
	end := 0
	for i := 0; i < chunkSize-1; i++ {
		start = i * (chunk + 1)
		end = start + chunk
		out = append(out, [2]int{start, end})
	}
	start = start + chunk + 1
	end = length - 1
	out = append(out, [2]int{start, end})
	return out
}

// parseRate parses bandwidth rate strings like "5M", "500K", "1G".
// Returns bytes per second. 0 means unlimited.
func parseRate(s string) (int64, error) {
	if s == "" || s == "0" {
		return 0, nil
	}

	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)

	multiplier := int64(1)
	numStr := upper

	switch {
	case strings.HasSuffix(upper, "G"):
		multiplier = 1024 * 1024 * 1024
		numStr = upper[:len(upper)-1]
	case strings.HasSuffix(upper, "M"):
		multiplier = 1024 * 1024
		numStr = upper[:len(upper)-1]
	case strings.HasSuffix(upper, "K"):
		multiplier = 1024
		numStr = upper[:len(upper)-1]
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rate: %s", s)
	}

	return int64(num * float64(multiplier)), nil
}

// formatBytes formats byte count to human readable string.
func formatBytes(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.1fGB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
```

**Step 2: Write tests in app/helpers_test.go**

```go
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
		{"application/pdf", ".pdf"},
		{"invalid/nope", "unknown"},
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
	// first chunk starts at 0
	if chunks[0][0] != 0 {
		t.Errorf("first chunk start = %d, want 0", chunks[0][0])
	}
	// last chunk ends at 99
	if chunks[len(chunks)-1][1] != 99 {
		t.Errorf("last chunk end = %d, want 99", chunks[len(chunks)-1][1])
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
```

**Step 3: Remove extracted functions from app.go**

Remove `isPiped`, `parseValidateURL`, `findExtension`, `getChunks` from `app/app.go`.
Change `c.getChunks(...)` calls to `getChunks(...)` (no longer a method).

**Step 4: Run tests**

Run: `cd /Users/vigo/Repos/Development/vigo/golang/cli/leech && go test ./app/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add app/helpers.go app/helpers_test.go app/app.go
git commit -m "refactor: extract helpers into helpers.go with tests"
```

---

### Task 3: Extract Resource and Download Logic into download.go

**Files:**
- Create: `app/download.go`
- Modify: `app/app.go` (remove extracted code)

**Step 1: Create app/download.go**

Move `resource` struct, `getResourceInformation`, `download`, `fetch` from `app/app.go`:

```go
package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type resource struct {
	chunks      [][2]int
	url         string
	filename    string
	contentType string
	length      int64
}

func (c *CLIApplication) getResourceInformation(url string) (*resource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: returned %d", errHTTPStatusIsNotOK, resp.StatusCode)
	}

	r := &resource{
		url:    url,
		length: resp.ContentLength,
	}

	acceptRanges, ok := resp.Header["Accept-Ranges"]
	if ok && resp.ContentLength > 0 && len(acceptRanges) > 0 && acceptRanges[0] == "bytes" {
		r.chunks = getChunks(int(resp.ContentLength), c.chunkSize)
	}

	if ct, ok := resp.Header["Content-Type"]; ok {
		r.contentType = ct[0]
	}

	if cd, ok := resp.Header["Content-Disposition"]; ok {
		_, params, err := mime.ParseMediaType(cd[0])
		if err == nil {
			r.filename = params["filename"]
		}
	}

	if r.filename == "" {
		basename := filepath.Base(url)
		r.filename = basename
		if r.contentType != "" {
			ext := findExtension(r.contentType)
			if ext != "unknown" {
				r.filename = basename + "." + ext
			}
		}
	}

	slog.Debug("resource info", "url", url, "length", r.length, "filename", r.filename, "chunks", len(r.chunks))

	return r, nil
}

func (c *CLIApplication) download(r *resource, done chan struct{}) {
	defer func() { done <- struct{}{} }()

	outputPath := filepath.Join(c.outputDir, r.filename)
	partPath := outputPath + ".part"

	if r.chunks != nil {
		var wg sync.WaitGroup
		fcontent := make([]byte, r.length)

		for i, chunkPair := range r.chunks {
			wg.Add(1)
			go func(part int, chunkPair [2]int) {
				defer wg.Done()
				byteParts, err := c.fetch(r.url, chunkPair)
				if err != nil {
					slog.Error("chunk download failed", "url", r.url, "part", part, "error", err)
					return
				}
				copy(fcontent[chunkPair[0]:], byteParts)
				slog.Debug("chunk downloaded", "url", r.url, "part", part)
			}(i, chunkPair)
		}
		wg.Wait()

		if err := os.WriteFile(partPath, fcontent, 0o644); err != nil {
			slog.Error("failed to write part file", "path", partPath, "error", err)
			return
		}
		if err := os.Rename(partPath, outputPath); err != nil {
			slog.Error("failed to rename part file", "path", partPath, "error", err)
			return
		}
	} else {
		// single chunk fallback - no Accept-Ranges support
		if err := c.downloadSingle(r, outputPath, partPath); err != nil {
			slog.Error("single download failed", "url", r.url, "error", err)
			return
		}
	}

	slog.Info("download complete", "file", r.filename, "size", formatBytes(r.length))
}

func (c *CLIApplication) downloadSingle(r *resource, outputPath, partPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	f, err := os.Create(partPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return os.Rename(partPath, outputPath)
}

func (c *CLIApplication) fetch(url string, chunk [2]int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Range", "bytes="+strconv.Itoa(chunk[0])+"-"+strconv.Itoa(chunk[1]))

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	slog.Debug("fetch response", "url", url, "status", resp.StatusCode, "range", fmt.Sprintf("%d-%d", chunk[0], chunk[1]))

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return b, nil
}
```

**Step 2: Update app.go to remove moved code, update struct**

`CLIApplication` struct should become:

```go
type CLIApplication struct {
	In        io.Reader
	Out       io.Writer
	URLS      []string
	Client    *http.Client
	chunkSize int
	outputDir string
	verbose   bool
	rateLimit int64
}
```

**Step 3: Verify it compiles**

Run: `go build ./...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add app/download.go app/app.go
git commit -m "refactor: extract download logic into download.go"
```

---

### Task 4: Structured Logging with log/slog

**Files:**
- Modify: `app/app.go` (setup slog in NewCLIApplication/Run)
- Modify: `app/download.go` (already uses slog from Task 3)

**Step 1: Add slog setup in Run()**

At the start of `Run()`, configure slog based on verbose flag:

```go
func (c *CLIApplication) setupLogging() {
	level := slog.LevelInfo
	if c.verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}
```

Call `c.setupLogging()` at the start of `Run()`.

**Step 2: Remove all fmt.Println debug statements from app.go**

Replace:
- `fmt.Println("optFlagVerbose", ...)` → remove
- `fmt.Println("firing ->", url)` → `slog.Debug("fetching resource info", "url", url)`
- `fmt.Println("-> err", err)` → `slog.Error("resource info failed", "url", url, "error", err)`
- `fmt.Println("downloadsCount", ...)` → `slog.Info("starting downloads", "count", downloadsCount)`
- All `fmt.Printf("%+v\n", r)` → `slog.Debug("resource", "url", r.url, "length", r.length)`
- `fmt.Println("Status Code", ...)` → already handled in Task 3
- `fmt.Println("save part", ...)` → already handled in Task 3

**Step 3: Verify it compiles and runs**

Run: `go build ./... && echo "http://example.com" | go run . -verbose`
Expected: debug-level slog output visible on stderr

**Step 4: Commit**

```bash
git add app/app.go
git commit -m "feat: add structured logging with log/slog"
```

---

### Task 5: Progress Bar (progressReader)

**Files:**
- Create: `app/progress.go`
- Create: `app/progress_test.go`

**Step 1: Write failing test in app/progress_test.go**

```go
package app

import (
	"bytes"
	"io"
	"strings"
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
		if err == io.EOF {
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
	bar := formatProgressBar(50, 100, 30)
	if !strings.Contains(bar, "50%") {
		t.Errorf("bar should contain 50%%, got %q", bar)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestProgress -v`
Expected: FAIL (undefined progressReader)

**Step 3: Implement app/progress.go**

```go
package app

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
)

type progressReader struct {
	reader     io.Reader
	total      int64
	read       atomic.Int64
	onProgress func(int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		current := pr.read.Add(int64(n))
		if pr.onProgress != nil {
			pr.onProgress(current)
		}
	}
	return n, err
}

// formatProgressBar renders: [████████░░░░░░░] 50% 5.0MB/10.0MB
func formatProgressBar(current, total int64, width int) string {
	if total <= 0 {
		return fmt.Sprintf("[%s] %s", strings.Repeat("?", width), formatBytes(current))
	}

	pct := float64(current) / float64(total)
	if pct > 1 {
		pct = 1
	}

	filled := int(pct * float64(width))
	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	return fmt.Sprintf("[%s] %3.0f%% %s/%s", bar, pct*100, formatBytes(current), formatBytes(total))
}
```

**Step 4: Run tests**

Run: `go test ./app/ -run TestProgress -v`
Expected: PASS

**Step 5: Commit**

```bash
git add app/progress.go app/progress_test.go
git commit -m "feat: add progress bar with progressReader"
```

---

### Task 6: Bandwidth Limiter (rateLimitedReader)

**Files:**
- Create: `app/ratelimit.go`
- Create: `app/ratelimit_test.go`

**Step 1: Write failing test in app/ratelimit_test.go**

```go
package app

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestRateLimitedReader(t *testing.T) {
	data := strings.Repeat("x", 10240) // 10KB
	r := strings.NewReader(data)

	limiter := newRateLimiter(5120) // 5KB/s
	lr := &rateLimitedReader{reader: r, limiter: limiter}

	start := time.Now()
	buf, err := io.ReadAll(lr)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if len(buf) != 10240 {
		t.Errorf("read %d bytes, want 10240", len(buf))
	}
	// 10KB at 5KB/s should take ~2s, allow margin
	if elapsed < 1*time.Second {
		t.Errorf("expected rate limiting, elapsed: %v", elapsed)
	}
}

func TestRateLimiterZeroMeansUnlimited(t *testing.T) {
	data := strings.Repeat("x", 1024)
	r := strings.NewReader(data)

	limiter := newRateLimiter(0) // unlimited
	lr := &rateLimitedReader{reader: r, limiter: limiter}

	buf, err := io.ReadAll(lr)
	if err != nil {
		t.Fatal(err)
	}
	if len(buf) != 1024 {
		t.Errorf("read %d bytes, want 1024", len(buf))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestRateLimit -v`
Expected: FAIL

**Step 3: Implement app/ratelimit.go**

```go
package app

import (
	"io"
	"sync"
	"time"
)

// rateLimiter implements a token bucket for bandwidth limiting.
// Shared across all goroutines for total bandwidth control.
type rateLimiter struct {
	mu         sync.Mutex
	rate       int64 // bytes per second, 0 = unlimited
	tokens     int64
	lastRefill time.Time
}

func newRateLimiter(bytesPerSecond int64) *rateLimiter {
	return &rateLimiter{
		rate:       bytesPerSecond,
		tokens:     bytesPerSecond,
		lastRefill: time.Now(),
	}
}

func (rl *rateLimiter) wait(n int) {
	if rl.rate == 0 {
		return // unlimited
	}

	for {
		rl.mu.Lock()

		now := time.Now()
		elapsed := now.Sub(rl.lastRefill)
		newTokens := int64(elapsed.Seconds() * float64(rl.rate))

		if newTokens > 0 {
			rl.tokens += newTokens
			if rl.tokens > rl.rate {
				rl.tokens = rl.rate
			}
			rl.lastRefill = now
		}

		if rl.tokens >= int64(n) {
			rl.tokens -= int64(n)
			rl.mu.Unlock()
			return
		}

		rl.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
}

type rateLimitedReader struct {
	reader  io.Reader
	limiter *rateLimiter
}

func (r *rateLimitedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.limiter.wait(n)
	}
	return n, err
}
```

**Step 4: Run tests**

Run: `go test ./app/ -run TestRateLimit -v -timeout 30s`
Expected: PASS

**Step 5: Commit**

```bash
git add app/ratelimit.go app/ratelimit_test.go
git commit -m "feat: add bandwidth limiter with token bucket"
```

---

### Task 7: Resume Support (.part files)

**Files:**
- Create: `app/resume.go`
- Create: `app/resume_test.go`

**Step 1: Write failing test in app/resume_test.go**

```go
package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetResumeOffset(t *testing.T) {
	dir := t.TempDir()
	partPath := filepath.Join(dir, "test.zip.part")

	// no .part file → offset 0
	offset := getResumeOffset(partPath)
	if offset != 0 {
		t.Errorf("expected 0 offset for missing file, got %d", offset)
	}

	// create .part file with 500 bytes
	if err := os.WriteFile(partPath, make([]byte, 500), 0o644); err != nil {
		t.Fatal(err)
	}

	offset = getResumeOffset(partPath)
	if offset != 500 {
		t.Errorf("expected 500 offset, got %d", offset)
	}
}

func TestCleanupPartFile(t *testing.T) {
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestResume -v && go test ./app/ -run TestCleanup -v`
Expected: FAIL

**Step 3: Implement app/resume.go**

```go
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
```

**Step 4: Run tests**

Run: `go test ./app/ -run "TestGetResume|TestCleanup" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add app/resume.go app/resume_test.go
git commit -m "feat: add resume support with .part files"
```

---

### Task 8: Update CLI Flags, Usage, and Wire Everything Together

**Files:**
- Modify: `app/usage.go`
- Modify: `app/app.go`

**Step 1: Update app/usage.go**

```go
package app

var cmdUsage = `
usage: %[1]s [-flags] URL URL URL ...

  flags:

  -version        display version information (%s)
  -verbose        verbose output / debug logging (default: false)
  -chunks N       chunk size for parallel download (default: 5)
  -limit RATE     bandwidth limit, e.g. 5M, 500K (default: 0, unlimited)
  -output DIR     output directory (default: current directory)

`
```

**Step 2: Rewrite app.go with new flag parsing and wiring**

```go
package app

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
)

var (
	errEmptyPipe         = errors.New("empty pipe")
	errEmptyURL          = errors.New("empty url")
	errInvalidURL        = errors.New("invalid url")
	errHTTPStatusIsNotOK = errors.New("http status is not ok")
)

const defaultChunkSize = 5

// CLIApplication represents the download manager instance.
type CLIApplication struct {
	In        io.Reader
	Out       io.Writer
	URLS      []string
	Client    *http.Client
	chunkSize int
	outputDir string
	verbose   bool
	rateLimit int64
	limiter   *rateLimiter
}

// NewCLIApplication creates and configures a new CLI app instance.
func NewCLIApplication() *CLIApplication {
	var (
		flagVersion   bool
		flagVerbose   bool
		flagChunkSize int
		flagLimit     string
		flagOutput    string
	)

	flag.BoolVar(&flagVersion, "version", false, "display version information ("+Version+")")
	flag.BoolVar(&flagVerbose, "verbose", false, "verbose output / debug logging")
	flag.IntVar(&flagChunkSize, "chunks", defaultChunkSize, "chunk size for parallel download")
	flag.StringVar(&flagLimit, "limit", "0", "bandwidth limit (e.g. 5M, 500K, 0=unlimited)")
	flag.StringVar(&flagOutput, "output", ".", "output directory")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, cmdUsage, os.Args[0], Version)
	}

	flag.Parse()

	if flagVersion {
		fmt.Fprintln(os.Stdout, Version)
		os.Exit(0)
	}

	rate, err := parseRate(flagLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid limit: %s\n", err)
		os.Exit(1)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true

	return &CLIApplication{
		In:        os.Stdin,
		Out:       os.Stdout,
		Client:    &http.Client{Transport: transport},
		chunkSize: flagChunkSize,
		outputDir: flagOutput,
		verbose:   flagVerbose,
		rateLimit: rate,
		limiter:   newRateLimiter(rate),
	}
}

func (c *CLIApplication) setupLogging() {
	level := slog.LevelInfo
	if c.verbose {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}

func (c *CLIApplication) parsePipe(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		url, err := parseValidateURL(scanner.Text())
		if err == nil {
			c.URLS = append(c.URLS, url)
		}
	}
	if len(c.URLS) == 0 {
		return errEmptyPipe
	}
	return nil
}

func (c *CLIApplication) parseArgs() {
	for _, arg := range flag.Args() {
		url, err := parseValidateURL(arg)
		if err == nil {
			c.URLS = append(c.URLS, url)
		}
	}
}

// Run executes the download manager.
func (c *CLIApplication) Run() error {
	c.setupLogging()

	if isPiped() {
		if err := c.parsePipe(c.In); err != nil {
			return err
		}
	}
	c.parseArgs()

	if len(c.URLS) == 0 {
		return errEmptyURL
	}

	// ensure output directory exists
	if err := os.MkdirAll(c.outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	slog.Info("starting downloads", "urls", len(c.URLS), "chunks", c.chunkSize, "limit", formatBytes(c.rateLimit)+"/s")

	resource := make(chan *resource)

	for _, u := range c.URLS {
		go func(url string) {
			slog.Debug("fetching resource info", "url", url)
			r, err := c.getResourceInformation(url)
			if err != nil {
				slog.Error("resource info failed", "url", url, "error", err)
				resource <- nil
				return
			}
			resource <- r
		}(u)
	}

	var downloadsCount int
	done := make(chan struct{})

	for range c.URLS {
		r := <-resource
		if r != nil {
			downloadsCount++
			go c.download(r, done)
		}
	}

	for i := 0; i < downloadsCount; i++ {
		<-done
	}

	slog.Info("all downloads complete", "count", downloadsCount)

	return nil
}
```

**Step 3: Verify it compiles and runs**

Run: `go build ./...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add app/app.go app/usage.go
git commit -m "feat: wire up all features, update CLI flags and usage"
```

---

### Task 9: Wire Progress and Rate Limiting into Download Pipeline

**Files:**
- Modify: `app/download.go`

**Step 1: Update download functions to use progressReader and rateLimitedReader**

In `downloadSingle`, wrap `resp.Body` with rate limiter and progress reader:

```go
func (c *CLIApplication) downloadSingle(r *resource, outputPath, partPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// check resume
	offset := getResumeOffset(partPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		slog.Info("resuming download", "file", r.filename, "offset", formatBytes(offset))
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	openFlag := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		openFlag |= os.O_APPEND
	} else {
		openFlag |= os.O_TRUNC
	}

	f, err := os.OpenFile(partPath, openFlag, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// wrap: body → rateLimiter → progressReader → file
	var reader io.Reader = resp.Body

	if c.limiter != nil {
		reader = &rateLimitedReader{reader: reader, limiter: c.limiter}
	}

	pr := &progressReader{
		reader: reader,
		total:  r.length,
		onProgress: func(n int64) {
			fmt.Fprintf(os.Stderr, "\r%s: %s", r.filename, formatProgressBar(offset+n, r.length, 30))
		},
	}

	if _, err := io.Copy(f, pr); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Fprintln(os.Stderr) // newline after progress bar

	return finalizePart(partPath, outputPath)
}
```

Update `fetch()` to use rate limiter:

```go
func (c *CLIApplication) fetch(url string, chunk [2]int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Range", "bytes="+strconv.Itoa(chunk[0])+"-"+strconv.Itoa(chunk[1]))

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var reader io.Reader = resp.Body
	if c.limiter != nil {
		reader = &rateLimitedReader{reader: reader, limiter: c.limiter}
	}

	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return b, nil
}
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add app/download.go
git commit -m "feat: wire progress bar and rate limiter into download pipeline"
```

---

### Task 10: golangci-lint v2 Configuration

**Files:**
- Replace: `.golangci.yml`

**Step 1: Write new .golangci.yml**

```yaml
version: "2"

run:
  concurrency: 4
  timeout: 5m

formatters:
  enable:
    - gofumpt
    - goimports
  settings:
    goimports:
      local-prefixes:
        - github.com/vigo/leech

linters:
  default: none
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gosec
    - revive
    - noctx
    - bodyclose
    - errorlint
    - nilerr
    - copyloopvar
    - intrange
    - perfsprint
    - unconvert
    - unparam
    - wastedassign
    - errname
    - canonicalheader
    - fatcontext

  settings:
    revive:
      enable-all-rules: true
      rules:
        - name: package-comments
          disabled: true
        - name: unused-parameter
          arguments:
            - allow-regex: "^_"
        - name: add-constant
          arguments:
            - max-lit-count: "3"
              allow-strs: '"","err"'
              allow-ints: "0,1,2,5,10,30,100"
              allow-floats: "1.0"
        - name: cognitive-complexity
          disabled: true
        - name: cyclomatic
          disabled: true
        - name: function-length
          arguments: [75, 0]
        - name: line-length-limit
          arguments: [120]

    govet:
      enable:
        - shadow

    perfsprint:
      strconcat: false

  exclusions:
    warn-unused: true
    presets:
      - std-error-handling
      - common-false-positives
    rules:
      - path: _test\.go
        linters:
          - bodyclose
          - errcheck
          - gosec
          - noctx
          - unparam
```

**Step 2: Verify lint passes**

Run: `cd /Users/vigo/Repos/Development/vigo/golang/cli/leech && golangci-lint run`
Expected: no errors (or fix any found)

**Step 3: Commit**

```bash
git add .golangci.yml
git commit -m "chore: migrate golangci-lint config to v2"
```

---

### Task 11: Pre-commit Configuration

**Files:**
- Create: `.pre-commit-config.yaml`

**Step 1: Create .pre-commit-config.yaml**

```yaml
fail_fast: true
repos:
  - repo: local
    hooks:
      - id: go-lint
        name: "Run golangci-lint"
        entry: golangci-lint run
        language: system
        pass_filenames: false
        files: \.go$

      - id: go-test
        name: "Run go tests"
        entry: go test -race ./...
        language: system
        pass_filenames: false
        files: \.go$
```

**Step 2: Verify pre-commit works**

Run: `cd /Users/vigo/Repos/Development/vigo/golang/cli/leech && pre-commit run --all-files`
Expected: both hooks pass

**Step 3: Commit**

```bash
git add .pre-commit-config.yaml
git commit -m "chore: add pre-commit hooks for go lint and test"
```

---

### Task 12: GitHub Actions (Lint + Test)

**Files:**
- Create: `.github/workflows/golint.yml`
- Create: `.github/workflows/gotests.yml`

**Step 1: Create .github/workflows/golint.yml**

```yaml
name: Run golangci-lint

on:
  pull_request:
    paths:
      - '**.go'
  push:
    branches:
      - main
    paths:
      - '**.go'

concurrency:
  group: leech-go-lint
  cancel-in-progress: true

jobs:
  lint:
    name: Run linter
    runs-on: ubuntu-24.04

    steps:
      - name: Checkout
        uses: actions/checkout@v6

      - name: Setup Go
        uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache-dependency-path: go.sum

      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v2.8
```

**Step 2: Create .github/workflows/gotests.yml**

```yaml
name: Run go tests

on:
  pull_request:
    paths:
      - '**.go'
  push:
    branches:
      - main
    tags-ignore:
      - '**'
    paths:
      - '**.go'

concurrency:
  group: leech-go-test
  cancel-in-progress: true

jobs:
  test:
    name: Run tests
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
        id: go

      - name: Run tests
        run: go test -v -race -failfast ./...
```

**Step 3: Commit**

```bash
git add .github/workflows/golint.yml .github/workflows/gotests.yml
git commit -m "ci: add GitHub Actions for lint and test"
```

---

### Task 13: Makefile

**Files:**
- Create: `Makefile`

**Step 1: Create Makefile**

```makefile
.PHONY: build test lint clean

APP_NAME := leech
VERSION := $(shell grep 'Version' app/version.go | cut -d'"' -f2)

build:
	go build -o $(APP_NAME) .

test:
	go test -v -race -failfast ./...

lint:
	golangci-lint run

clean:
	rm -f $(APP_NAME)
	rm -f *.part

install:
	go install .
```

**Step 2: Verify**

Run: `make build && make test && make lint`
Expected: all pass

**Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile"
```

---

### Task 14: Final Integration Test and Cleanup

**Files:**
- All files from previous tasks

**Step 1: Run full test suite**

Run: `go test -v -race -failfast ./...`
Expected: PASS

**Step 2: Run linter**

Run: `golangci-lint run`
Expected: no errors

**Step 3: Build and smoke test**

Run: `go build -o leech . && echo "https://www.google.com/robots.txt" | ./leech -verbose`
Expected: downloads robots.txt with debug logging

**Step 4: Test with multiple URLs**

Run: `./leech -verbose https://www.google.com/robots.txt https://www.google.com/favicon.ico`
Expected: downloads both files concurrently

**Step 5: Final commit if any fixes needed**

```bash
git add -A
git commit -m "chore: final cleanup and integration verification"
```
