package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
)

func newTestServer(content []byte, supportRanges bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.Header().Set("Content-Type", "application/octet-stream")
			if supportRanges {
				w.Header().Set("Accept-Ranges", "bytes")
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			rangeHeader := r.Header.Get("Range")
			if rangeHeader != "" && supportRanges {
				var start, end int
				n, _ := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
				if n == 1 {
					end = len(content) - 1
				}
				if end >= len(content) {
					end = len(content) - 1
				}
				w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
				w.WriteHeader(http.StatusPartialContent)
				w.Write(content[start : end+1])
			} else {
				w.Header().Set("Content-Length", strconv.Itoa(len(content)))
				w.Write(content)
			}
		}
	}))
}

func TestGetResourceInformation(t *testing.T) {
	content := []byte("hello world test content")
	ts := newTestServer(content, true)
	defer ts.Close()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 3,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/testfile")
	if err != nil {
		t.Fatal(err)
	}

	if r.length != int64(len(content)) {
		t.Errorf("length = %d, want %d", r.length, len(content))
	}
	if len(r.chunks) != 3 {
		t.Errorf("chunks = %d, want 3", len(r.chunks))
	}
	if r.filename != "testfile.bin" {
		t.Errorf("filename = %q, want 'testfile.bin'", r.filename)
	}
}

func TestGetResourceInformationNoRanges(t *testing.T) {
	content := []byte("no ranges support")
	ts := newTestServer(content, false)
	defer ts.Close()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 5,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/file.bin")
	if err != nil {
		t.Fatal(err)
	}

	if r.chunks != nil {
		t.Error("expected nil chunks for server without Accept-Ranges")
	}
}

func TestGetResourceInformation404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 5,
	}

	_, err := app.getResourceInformation(context.Background(), ts.URL+"/missing")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestGetResourceInformationQueryParams(t *testing.T) {
	content := []byte("signed url content")
	ts := newTestServer(content, false)
	defer ts.Close()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 5,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/file.zip?X-Amz-Signature=abc123&expires=999")
	if err != nil {
		t.Fatal(err)
	}

	if r.filename != "file.zip" {
		t.Errorf("filename = %q, want 'file.zip' (query params should be stripped)", r.filename)
	}
}

func TestFetchToFile(t *testing.T) {
	content := []byte("0123456789abcdef")
	ts := newTestServer(content, true)
	defer ts.Close()

	app := &CLIApplication{
		Client:  ts.Client(),
		limiter: newRateLimiter(0),
	}

	f, err := os.CreateTemp(t.TempDir(), "fetch-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	if err := f.Truncate(int64(len(content))); err != nil {
		t.Fatal(err)
	}

	var counter atomic.Int64
	if err := app.fetchToFile(context.Background(), ts.URL+"/file.bin", [2]int64{0, 7}, f, &counter); err != nil {
		t.Fatal(err)
	}

	got := make([]byte, 8)
	if _, err := f.ReadAt(got, 0); err != nil {
		t.Fatal(err)
	}
	if string(got) != "01234567" {
		t.Errorf("fetchToFile wrote %q, want '01234567'", string(got))
	}
	if counter.Load() != 8 {
		t.Errorf("counter = %d, want 8", counter.Load())
	}
}

func TestFetchToFileMiddleChunk(t *testing.T) {
	content := []byte("0123456789abcdef")
	ts := newTestServer(content, true)
	defer ts.Close()

	app := &CLIApplication{
		Client:  ts.Client(),
		limiter: newRateLimiter(0),
	}

	f, err := os.CreateTemp(t.TempDir(), "fetch-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	if err := f.Truncate(int64(len(content))); err != nil {
		t.Fatal(err)
	}

	var counter atomic.Int64
	if err := app.fetchToFile(context.Background(), ts.URL+"/file.bin", [2]int64{4, 9}, f, &counter); err != nil {
		t.Fatal(err)
	}

	got := make([]byte, 6)
	if _, err := f.ReadAt(got, 4); err != nil {
		t.Fatal(err)
	}
	if string(got) != "456789" {
		t.Errorf("fetchToFile wrote %q, want '456789'", string(got))
	}
	if counter.Load() != 6 {
		t.Errorf("counter = %d, want 6", counter.Load())
	}
}

func TestDownloadSingle(t *testing.T) {
	content := []byte("single chunk download content here")
	ts := newTestServer(content, false)
	defer ts.Close()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "output.bin")
	partPath := outputPath + ".part"

	app := &CLIApplication{
		Client:    ts.Client(),
		limiter:   newRateLimiter(0),
		outputDir: dir,
	}

	r := &resource{
		url:      ts.URL + "/file.bin",
		filename: "output.bin",
		length:   int64(len(content)),
	}

	var counter atomic.Int64
	err := app.downloadSingle(context.Background(), r, outputPath, partPath, &counter)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded content = %q, want %q", string(got), string(content))
	}
}

func TestDownloadChunked(t *testing.T) {
	content := []byte("abcdefghijklmnopqrstuvwxyz0123456789")
	ts := newTestServer(content, true)
	defer ts.Close()

	dir := t.TempDir()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 3,
		limiter:   newRateLimiter(0),
		outputDir: dir,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/alphabet.bin")
	if err != nil {
		t.Fatal(err)
	}

	pd := newProgressDisplay()
	done := make(chan downloadResult, 1)
	go app.download(context.Background(), r, done, pd)
	<-done

	outputPath := filepath.Join(dir, r.filename)
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded content length = %d, want %d", len(got), len(content))
	}
}

func TestDownloadSingleNoChunks(t *testing.T) {
	content := []byte("single download via download func")
	ts := newTestServer(content, false)
	defer ts.Close()

	dir := t.TempDir()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 5,
		limiter:   newRateLimiter(0),
		outputDir: dir,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/single.bin")
	if err != nil {
		t.Fatal(err)
	}

	pd2 := newProgressDisplay()
	done := make(chan downloadResult, 1)
	go app.download(context.Background(), r, done, pd2)
	<-done

	outputPath := filepath.Join(dir, r.filename)
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded content = %q, want %q", string(got), string(content))
	}
}

func TestDownloadChunkedFallbackToSingle(t *testing.T) {
	content := []byte("fallback content here!!")

	// server advertises Accept-Ranges on HEAD but rejects range GETs
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if r.Header.Get("Range") != "" {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.Write(content)
		}
	}))
	defer ts.Close()

	dir := t.TempDir()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 3,
		limiter:   newRateLimiter(0),
		outputDir: dir,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/fallback.bin")
	if err != nil {
		t.Fatal(err)
	}

	if r.chunks == nil {
		t.Fatal("expected chunks to be set (server advertises Accept-Ranges)")
	}

	pd := newProgressDisplay()
	done := make(chan downloadResult, 1)
	go app.download(context.Background(), r, done, pd)
	result := <-done

	if !result.ok {
		t.Error("expected download to succeed via single-stream fallback")
	}

	outputPath := filepath.Join(dir, r.filename)
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded content = %q, want %q", string(got), string(content))
	}
}

func TestGetResourceInformationContentDisposition(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="report.pdf"`)
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Write([]byte("hello"))
	}))
	defer ts.Close()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 5,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/download")
	if err != nil {
		t.Fatal(err)
	}

	if r.filename != "report.pdf" {
		t.Errorf("filename = %q, want 'report.pdf'", r.filename)
	}
}

func TestGetResourceInformationNoContentType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		w.Header().Del("Content-Type")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Write([]byte("hello"))
	}))
	defer ts.Close()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 5,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/plain.dat")
	if err != nil {
		t.Fatal(err)
	}

	// filename should be the basename from URL path
	if r.filename != "plain.dat" {
		t.Errorf("filename = %q, want 'plain.dat'", r.filename)
	}
}

// --- Grup 2 kısmi: küçük boşluklar ---

func TestFetchToFileNon206(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // 200 instead of 206
		w.Write([]byte("data"))
	}))
	defer ts.Close()

	app := &CLIApplication{
		Client:  ts.Client(),
		limiter: newRateLimiter(0),
	}

	f, err := os.CreateTemp(t.TempDir(), "fetch-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	var counter atomic.Int64
	err = app.fetchToFile(context.Background(), ts.URL+"/file.bin", [2]int64{0, 7}, f, &counter)
	if err == nil {
		t.Error("expected error for non-206 response")
	}
}

func TestFetchToFileCancelledContext(t *testing.T) {
	ts := newTestServer([]byte("test data"), true)
	defer ts.Close()

	app := &CLIApplication{
		Client:  ts.Client(),
		limiter: newRateLimiter(0),
	}

	f, err := os.CreateTemp(t.TempDir(), "fetch-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var counter atomic.Int64
	err = app.fetchToFile(ctx, ts.URL+"/file.bin", [2]int64{0, 3}, f, &counter)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestGetResourceInformationRootURL(t *testing.T) {
	content := []byte("root content")
	ts := newTestServer(content, false)
	defer ts.Close()

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 5,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/")
	if err != nil {
		t.Fatal(err)
	}

	// root URL should fall back to "download" basename + extension from content type
	if r.filename != "download.bin" {
		t.Errorf("filename = %q, want 'download.bin' for root URL", r.filename)
	}
}

// --- Grup 6: downloadChunked/Single error path'leri ---

func TestDownloadChunkedOpenFileError(t *testing.T) {
	content := []byte("test content for chunked")
	ts := newTestServer(content, true)
	defer ts.Close()

	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}

	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 3,
		limiter:   newRateLimiter(0),
		outputDir: readOnlyDir,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/file.bin")
	if err != nil {
		t.Fatal(err)
	}

	var downloaded atomic.Int64
	outputPath := filepath.Join(readOnlyDir, "file.bin")
	partPath := outputPath + ".part"
	ok := app.downloadChunked(context.Background(), r, outputPath, partPath, &downloaded)
	if ok {
		t.Error("expected downloadChunked to fail with read-only dir")
	}
}

func TestDownloadSingle500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	dir := t.TempDir()
	app := &CLIApplication{
		Client:  ts.Client(),
		limiter: newRateLimiter(0),
	}

	r := &resource{url: ts.URL + "/file.bin", filename: "file.bin", length: 100}

	var counter atomic.Int64
	err := app.downloadSingle(
		context.Background(), r,
		filepath.Join(dir, "file.bin"), filepath.Join(dir, "file.bin.part"),
		&counter,
	)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestDownloadSingleResumeRangeIgnored(t *testing.T) {
	content := []byte("full content here")
	// server ignores Range header, returns 200
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer ts.Close()

	dir := t.TempDir()
	partPath := filepath.Join(dir, "file.bin.part")
	outputPath := filepath.Join(dir, "file.bin")

	// create partial file to trigger resume logic
	if err := os.WriteFile(partPath, []byte("partial"), permFile); err != nil {
		t.Fatal(err)
	}

	app := &CLIApplication{
		Client:  ts.Client(),
		limiter: newRateLimiter(0),
	}

	r := &resource{url: ts.URL + "/file.bin", filename: "file.bin", length: int64(len(content))}

	var counter atomic.Int64
	err := app.downloadSingle(context.Background(), r, outputPath, partPath, &counter)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestDownloadSingleCancelledContext(t *testing.T) {
	ts := newTestServer([]byte("data"), false)
	defer ts.Close()

	dir := t.TempDir()
	app := &CLIApplication{
		Client:  ts.Client(),
		limiter: newRateLimiter(0),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &resource{url: ts.URL + "/file.bin", filename: "file.bin", length: 4}

	var counter atomic.Int64
	err := app.downloadSingle(ctx, r,
		filepath.Join(dir, "file.bin"), filepath.Join(dir, "file.bin.part"),
		&counter,
	)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// --- Grup 7: download orchestrator error path'leri ---

func TestDownloadChunkedFailSingleFallbackFail(t *testing.T) {
	// HEAD returns 200 with Accept-Ranges, all GETs return 500
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "100")
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)

			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	dir := t.TempDir()
	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 3,
		limiter:   newRateLimiter(0),
		outputDir: dir,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/file.bin")
	if err != nil {
		t.Fatal(err)
	}

	pd := newProgressDisplay()
	done := make(chan downloadResult, 1)
	go app.download(context.Background(), r, done, pd)
	result := <-done

	if result.ok {
		t.Error("expected download to fail")
	}
}

func TestDownloadNoChunksSingleFail(t *testing.T) {
	// HEAD returns 200 without Accept-Ranges, GET returns 500
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "100")
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)

			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	dir := t.TempDir()
	app := &CLIApplication{
		Client:    ts.Client(),
		chunkSize: 5,
		limiter:   newRateLimiter(0),
		outputDir: dir,
	}

	r, err := app.getResourceInformation(context.Background(), ts.URL+"/file.bin")
	if err != nil {
		t.Fatal(err)
	}

	pd := newProgressDisplay()
	done := make(chan downloadResult, 1)
	go app.download(context.Background(), r, done, pd)
	result := <-done

	if result.ok {
		t.Error("expected download to fail")
	}
}
