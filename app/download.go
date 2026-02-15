package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	progressUpdateInterval = 200 * time.Millisecond
	logKeyURL              = "url"
	logKeyError            = "error"
)

type resource struct {
	chunks      [][2]int64
	url         string
	filename    string
	contentType string
	length      int64
}

type downloadResult struct {
	size int64
	ok   bool
}

func (c *CLIApplication) getResourceInformation(ctx context.Context, url string) (*resource, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
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
		r.chunks = getChunks(resp.ContentLength, c.chunkSize)
	}

	if ct, ok := resp.Header["Content-Type"]; ok {
		r.contentType = ct[0]
	}

	if cd, ok := resp.Header["Content-Disposition"]; ok {
		_, params, err := mime.ParseMediaType(cd[0])
		if err == nil {
			name := filepath.Base(params["filename"])
			if name != "." && name != "/" {
				r.filename = name
			}
		}
	}

	if r.filename == "" {
		parsed, _ := neturl.Parse(r.url)
		basename := filepath.Base(parsed.Path)

		if basename == "" || basename == "." || basename == "/" {
			basename = "download"
		}

		r.filename = basename
		if r.contentType != "" && filepath.Ext(basename) == "" {
			ext := findExtension(r.contentType)
			if ext != "unknown" {
				r.filename = basename + "." + ext
			}
		}
	}

	slog.Debug("resource info", logKeyURL, url, "length", r.length, "filename", r.filename, "chunks", len(r.chunks))

	return r, nil
}

func (c *CLIApplication) download(ctx context.Context, r *resource, done chan downloadResult, pd *progressDisplay) {
	var success bool
	defer func() {
		var size int64
		if success {
			size = max(r.length, 0)
		}
		done <- downloadResult{size: size, ok: success}
	}()

	outputPath := filepath.Join(c.outputDir, r.filename)
	partPath := outputPath + ".part"

	if r.chunks != nil {
		if ok := c.downloadChunked(ctx, r, outputPath, partPath, pd); !ok {
			slog.Warn("chunked download failed, falling back to single stream", logKeyURL, r.url)
			_ = os.Remove(partPath)

			if err := c.downloadSingle(ctx, r, outputPath, partPath, pd); err != nil {
				slog.Error("single download fallback failed", logKeyURL, r.url, logKeyError, err)
				return
			}
		}
	} else {
		if err := c.downloadSingle(ctx, r, outputPath, partPath, pd); err != nil {
			slog.Error("single download failed", logKeyURL, r.url, logKeyError, err)
			return
		}
	}

	success = true

	slog.Info("download complete", "file", r.filename, "size", formatBytes(r.length))
}

func (c *CLIApplication) downloadChunked(
	ctx context.Context, r *resource, outputPath, partPath string, pd *progressDisplay,
) bool {
	var wg sync.WaitGroup
	var downloaded atomic.Int64
	var errMu sync.Mutex
	var fetchErr error

	chunkCtx, chunkCancel := context.WithCancel(ctx)
	defer chunkCancel()

	pd.add(r.filename, &downloaded, r.length)

	f, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY, permFile)
	if err != nil {
		slog.Error("failed to create part file", "path", partPath, logKeyError, err)
		return false
	}

	// only truncate if file size doesn't match expected length
	info, statErr := f.Stat()
	if statErr != nil || info.Size() != r.length {
		if err := f.Truncate(r.length); err != nil {
			_ = f.Close()
			slog.Error("failed to allocate part file", "path", partPath, logKeyError, err)
			return false
		}
	}

	for i, chunkPair := range r.chunks {
		wg.Go(func() {
			if err := c.fetchToFile(chunkCtx, r.url, chunkPair, f, &downloaded); err != nil {
				slog.Error("chunk download failed", logKeyURL, r.url, "part", i, logKeyError, err)
				errMu.Lock()
				fetchErr = err
				errMu.Unlock()
				chunkCancel()

				return
			}

			slog.Debug("chunk downloaded", logKeyURL, r.url, "part", i)
		})
	}
	wg.Wait()
	_ = f.Close()

	if fetchErr != nil {
		return false
	}

	if err := finalizePart(partPath, outputPath); err != nil {
		slog.Error("failed to finalize download", "path", partPath, logKeyError, err)
		return false
	}

	return true
}

func (c *CLIApplication) downloadSingle(
	ctx context.Context, r *resource, outputPath, partPath string, pd *progressDisplay,
) error {
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

	// reject non-success responses
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("%w: returned %d", errHTTPStatusIsNotOK, resp.StatusCode)
	}

	// if server didn't honor Range request, restart from scratch
	if offset > 0 && resp.StatusCode != http.StatusPartialContent {
		slog.Info("server ignored range request, restarting", "file", r.filename)
		offset = 0
	}

	openFlag := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		openFlag |= os.O_APPEND
	} else {
		openFlag |= os.O_TRUNC
	}

	f, err := os.OpenFile(partPath, openFlag, permFile)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var downloaded atomic.Int64
	downloaded.Store(offset)
	pd.add(r.filename, &downloaded, r.length)

	var reader io.Reader = resp.Body

	if c.limiter != nil {
		reader = &rateLimitedReader{reader: reader, limiter: c.limiter}
	}

	reader = &countingReader{reader: reader, counter: &downloaded}

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return finalizePart(partPath, outputPath)
}

func (c *CLIApplication) fetchToFile(
	ctx context.Context, url string, chunk [2]int64, f *os.File, downloaded *atomic.Int64,
) error {
	start, end := chunk[0], chunk[1]
	chunkBytes := end - start + 1

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chunk fetch failed: %w: returned %d", errHTTPStatusIsNotOK, resp.StatusCode)
	}

	slog.Debug("fetch response", logKeyURL, url, "status", resp.StatusCode, "range", fmt.Sprintf("%d-%d", start, end))

	var reader io.Reader = resp.Body
	if c.limiter != nil {
		reader = &rateLimitedReader{reader: reader, limiter: c.limiter}
	}

	reader = &countingReader{reader: reader, counter: downloaded}

	writer := io.NewOffsetWriter(f, start)

	written, err := io.Copy(writer, reader)
	if err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}

	if written != chunkBytes {
		return fmt.Errorf("chunk size mismatch: got %d bytes, want %d", written, chunkBytes)
	}

	return nil
}
