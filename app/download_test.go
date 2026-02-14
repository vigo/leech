package app

import (
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
				fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
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

	r, err := app.getResourceInformation(ts.URL + "/testfile")
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

	r, err := app.getResourceInformation(ts.URL + "/file.bin")
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

	_, err := app.getResourceInformation(ts.URL + "/missing")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestFetch(t *testing.T) {
	content := []byte("0123456789abcdef")
	ts := newTestServer(content, true)
	defer ts.Close()

	app := &CLIApplication{
		Client:  ts.Client(),
		limiter: newRateLimiter(0),
	}

	var counter atomic.Int64
	data, err := app.fetch(ts.URL+"/file.bin", [2]int{0, 7}, &counter)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "01234567" {
		t.Errorf("fetch returned %q, want '01234567'", string(data))
	}
	if counter.Load() != 8 {
		t.Errorf("counter = %d, want 8", counter.Load())
	}
}

func TestFetchMiddleChunk(t *testing.T) {
	content := []byte("0123456789abcdef")
	ts := newTestServer(content, true)
	defer ts.Close()

	app := &CLIApplication{
		Client:  ts.Client(),
		limiter: newRateLimiter(0),
	}

	var counter atomic.Int64
	data, err := app.fetch(ts.URL+"/file.bin", [2]int{4, 9}, &counter)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "456789" {
		t.Errorf("fetch returned %q, want '456789'", string(data))
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

	pd := newProgressDisplay()
	err := app.downloadSingle(r, outputPath, partPath, pd)
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

	r, err := app.getResourceInformation(ts.URL + "/alphabet.bin")
	if err != nil {
		t.Fatal(err)
	}

	pd := newProgressDisplay()
	done := make(chan downloadResult, 1)
	go app.download(r, done, pd)
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

	r, err := app.getResourceInformation(ts.URL + "/single.bin")
	if err != nil {
		t.Fatal(err)
	}

	pd2 := newProgressDisplay()
	done := make(chan downloadResult, 1)
	go app.download(r, done, pd2)
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

	r, err := app.getResourceInformation(ts.URL + "/download")
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

	r, err := app.getResourceInformation(ts.URL + "/plain.dat")
	if err != nil {
		t.Fatal(err)
	}

	// filename should be the basename from URL path
	if r.filename != "plain.dat" {
		t.Errorf("filename = %q, want 'plain.dat'", r.filename)
	}
}
