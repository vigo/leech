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
	"sync/atomic"
	"time"
)

const (
	progressUpdateInterval = 200 * time.Millisecond
	timeoutSafetyFactor    = 3
	logKeyURL              = "url"
	logKeyError            = "error"
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
			name := filepath.Base(params["filename"])
			if name != "." && name != "/" {
				r.filename = name
			}
		}
	}

	if r.filename == "" {
		basename := filepath.Base(url)
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

func (c *CLIApplication) download(r *resource, done chan int64, pd *progressDisplay) {
	defer func() { done <- r.length }()

	outputPath := filepath.Join(c.outputDir, r.filename)
	partPath := outputPath + ".part"

	if r.chunks != nil {
		var wg sync.WaitGroup
		var downloaded atomic.Int64
		var errMu sync.Mutex
		var fetchErr error

		pd.add(r.filename, &downloaded, r.length)

		f, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, permFile)
		if err != nil {
			slog.Error("failed to create part file", "path", partPath, logKeyError, err)
			return
		}

		// pre-allocate file size so concurrent WriteAt calls work
		if err := f.Truncate(r.length); err != nil {
			_ = f.Close()
			slog.Error("failed to allocate part file", "path", partPath, logKeyError, err)
			return
		}

		for i, chunkPair := range r.chunks {
			wg.Go(func() {
				byteParts, err := c.fetch(r.url, chunkPair, &downloaded)
				if err != nil {
					slog.Error("chunk download failed", logKeyURL, r.url, "part", i, logKeyError, err)
					errMu.Lock()
					fetchErr = err
					errMu.Unlock()

					return
				}

				expected := chunkPair[1] - chunkPair[0] + 1
				if len(byteParts) != expected {
					slog.Error("chunk size mismatch",
						logKeyURL, r.url, "part", i, "got", len(byteParts), "want", expected,
					)
					errMu.Lock()
					fetchErr = fmt.Errorf("chunk %d: got %d bytes, want %d", i, len(byteParts), expected)
					errMu.Unlock()

					return
				}

				if _, err := f.WriteAt(byteParts, int64(chunkPair[0])); err != nil {
					slog.Error("chunk write failed", logKeyURL, r.url, "part", i, logKeyError, err)
					errMu.Lock()
					fetchErr = err
					errMu.Unlock()

					return
				}

				slog.Debug("chunk downloaded", logKeyURL, r.url, "part", i)
			})
		}
		wg.Wait()
		_ = f.Close()

		if fetchErr != nil {
			slog.Error("download aborted due to chunk failure", logKeyURL, r.url, logKeyError, fetchErr)
			_ = os.Remove(partPath)

			return
		}

		if err := finalizePart(partPath, outputPath); err != nil {
			slog.Error("failed to finalize download", "path", partPath, logKeyError, err)
			return
		}
	} else {
		if err := c.downloadSingle(r, outputPath, partPath, pd); err != nil {
			slog.Error("single download failed", logKeyURL, r.url, logKeyError, err)
			return
		}
	}

	slog.Info("download complete", "file", r.filename, "size", formatBytes(r.length))
}

func (c *CLIApplication) downloadSingle(r *resource, outputPath, partPath string, pd *progressDisplay) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

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

func (c *CLIApplication) fetch(url string, chunk [2]int, downloaded *atomic.Int64) ([]byte, error) {
	start, end := chunk[0], chunk[1]
	chunkBytes := int64(end - start + 1)

	timeout := 30 * time.Second
	if c.limiter != nil && c.limiter.rate > 0 {
		expected := time.Duration(chunkBytes/c.limiter.rate+1) * time.Second
		if needed := expected * timeoutSafetyFactor; needed > timeout {
			timeout = needed
		}
	}
	if timeout > 30*time.Minute {
		timeout = 30 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Range", "bytes="+strconv.Itoa(start)+"-"+strconv.Itoa(end))

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chunk fetch failed: %w: returned %d", errHTTPStatusIsNotOK, resp.StatusCode)
	}

	slog.Debug("fetch response", logKeyURL, url, "status", resp.StatusCode, "range", fmt.Sprintf("%d-%d", start, end))

	var reader io.Reader = resp.Body
	if c.limiter != nil {
		reader = &rateLimitedReader{reader: reader, limiter: c.limiter}
	}

	reader = &countingReader{reader: reader, counter: downloaded}

	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return b, nil
}
