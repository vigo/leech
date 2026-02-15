package app

import (
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func TestParsePipe(t *testing.T) {
	app := &CLIApplication{}

	input := "https://example.com/file1.zip\nhttps://example.com/file2.zip\nnot-a-url\n"
	err := app.parsePipe(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(app.URLS) != 2 {
		t.Errorf("expected 2 URLs, got %d", len(app.URLS))
	}
}

func TestParsePipeMultipleValidURLs(t *testing.T) {
	app := &CLIApplication{}

	input := "https://example.com/a.zip\nhttp://example.com/b.tar.gz\nhttps://cdn.example.com/c.bin\n"
	err := app.parsePipe(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(app.URLS) != 3 {
		t.Errorf("expected 3 URLs, got %d", len(app.URLS))
	}
	expected := []string{
		"https://example.com/a.zip",
		"http://example.com/b.tar.gz",
		"https://cdn.example.com/c.bin",
	}
	for i, want := range expected {
		if app.URLS[i] != want {
			t.Errorf("URLS[%d] = %q, want %q", i, app.URLS[i], want)
		}
	}
}

func TestParsePipeEmpty(t *testing.T) {
	app := &CLIApplication{}

	err := app.parsePipe(strings.NewReader("not-a-url\n"))
	if err == nil {
		t.Error("expected error for empty pipe with no valid URLs")
	}
}

func TestParsePipeAllInvalid(t *testing.T) {
	app := &CLIApplication{}

	err := app.parsePipe(strings.NewReader("ftp://bad\nno-scheme\n"))
	if err == nil {
		t.Error("expected error for pipe with only invalid URLs")
	}
}

func TestParsePipeCRLineEndings(t *testing.T) {
	app := &CLIApplication{}

	input := "https://example.com/file1.zip\rhttps://example.com/file2.zip"
	err := app.parsePipe(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(app.URLS) != 2 {
		t.Errorf("expected 2 URLs with CR line endings, got %d", len(app.URLS))
	}
}

func TestParsePipeCRLFLineEndings(t *testing.T) {
	app := &CLIApplication{}

	input := "https://example.com/file1.zip\r\nhttps://example.com/file2.zip\r\n"
	err := app.parsePipe(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(app.URLS) != 2 {
		t.Errorf("expected 2 URLs with CRLF line endings, got %d", len(app.URLS))
	}
}

func TestParsePipeWhitespace(t *testing.T) {
	app := &CLIApplication{}

	input := "  https://example.com/file1.zip  \n\n  https://example.com/file2.zip\n\n"
	err := app.parsePipe(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(app.URLS) != 2 {
		t.Errorf("expected 2 URLs with whitespace, got %d", len(app.URLS))
	}
}

func TestParseArgs(t *testing.T) {
	app := &CLIApplication{}

	args := []string{
		"https://example.com/file1.zip",
		"https://example.com/file2.zip",
		"https://example.com/file3.zip",
	}
	app.parseArgs(args)

	if len(app.URLS) != 3 {
		t.Errorf("expected 3 URLs, got %d", len(app.URLS))
	}
	for i, want := range args {
		if app.URLS[i] != want {
			t.Errorf("URLS[%d] = %q, want %q", i, app.URLS[i], want)
		}
	}
}

func TestParseArgsMixed(t *testing.T) {
	app := &CLIApplication{}

	args := []string{
		"https://example.com/valid.zip",
		"not-a-url",
		"ftp://invalid-scheme.com/file",
		"http://example.com/also-valid.tar.gz",
	}
	app.parseArgs(args)

	if len(app.URLS) != 2 {
		t.Errorf("expected 2 valid URLs, got %d", len(app.URLS))
	}
}

func TestParseArgsEmpty(t *testing.T) {
	app := &CLIApplication{}

	app.parseArgs([]string{})

	if len(app.URLS) != 0 {
		t.Errorf("expected 0 URLs, got %d", len(app.URLS))
	}
}

func TestParsePipeThenArgs(t *testing.T) {
	app := &CLIApplication{}

	// simulate: cat urls.txt | leech extra-url
	pipeInput := "https://example.com/from-pipe.zip\n"
	err := app.parsePipe(strings.NewReader(pipeInput))
	if err != nil {
		t.Fatal(err)
	}

	app.parseArgs([]string{"https://example.com/from-args.zip"})

	if len(app.URLS) != 2 {
		t.Errorf("expected 2 URLs (pipe + args), got %d", len(app.URLS))
	}
	if app.URLS[0] != "https://example.com/from-pipe.zip" {
		t.Errorf("URLS[0] = %q, want pipe URL", app.URLS[0])
	}
	if app.URLS[1] != "https://example.com/from-args.zip" {
		t.Errorf("URLS[1] = %q, want args URL", app.URLS[1])
	}
}

func TestSetupLogging(t *testing.T) {
	app := &CLIApplication{verbose: true}
	app.setupLogging()

	app2 := &CLIApplication{verbose: false}
	app2.setupLogging()
}

func TestNewCLIApplication(t *testing.T) {
	app := NewCLIApplication()
	if app == nil {
		t.Fatal("NewCLIApplication() returned nil")
	}
	if app.In == nil {
		t.Error("expected In to be set")
	}
	if app.Out == nil {
		t.Error("expected Out to be set")
	}
	if app.Client == nil {
		t.Error("expected Client to be set")
	}
}

func TestParsePipeReaderError(t *testing.T) {
	app := &CLIApplication{}
	err := app.parsePipe(iotest.ErrReader(errors.New("read error")))
	if err == nil {
		t.Error("expected error from ErrReader")
	}
}

// --- Grup 4: parseFlags ---

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantErr   bool
		errTarget error
		checkFunc func(*CLIApplication) error
	}{
		{
			name:      "version flag",
			args:      []string{"leech", "-version"},
			wantErr:   true,
			errTarget: errVersionRequested,
		},
		{
			name: "defaults",
			args: []string{"leech"},
			checkFunc: func(c *CLIApplication) error {
				if c.chunkSize != defaultChunkSize {
					return errors.New("chunkSize mismatch")
				}
				if c.outputDir != "." {
					return errors.New("outputDir mismatch")
				}
				if c.verbose {
					return errors.New("verbose should be false")
				}
				return nil
			},
		},
		{
			name: "custom flags",
			args: []string{"leech", "-verbose", "-chunks", "10", "-output", "/tmp", "-limit", "5M"},
			checkFunc: func(c *CLIApplication) error {
				if c.chunkSize != 10 {
					return errors.New("chunkSize mismatch")
				}
				if c.outputDir != "/tmp" {
					return errors.New("outputDir mismatch")
				}
				if !c.verbose {
					return errors.New("verbose should be true")
				}
				return nil
			},
		},
		{
			name:    "invalid limit",
			args:    []string{"leech", "-limit", "abc"},
			wantErr: true,
		},
		{
			name:    "chunks too low",
			args:    []string{"leech", "-chunks", "0"},
			wantErr: true,
		},
		{
			name:    "chunks too high",
			args:    []string{"leech", "-chunks", "100"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag.CommandLine = flag.NewFlagSet(tt.args[0], flag.ContinueOnError)

			oldArgs := os.Args
			os.Args = tt.args
			defer func() { os.Args = oldArgs }()

			app := &CLIApplication{Out: io.Discard}
			err := app.parseFlags()

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.errTarget != nil && !errors.Is(err, tt.errTarget) {
					t.Errorf("error = %v, want %v", err, tt.errTarget)
				}
				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if tt.checkFunc != nil {
				if err := tt.checkFunc(app); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

// --- Grup 5: Run integration ---

func TestRunVersionFlag(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("leech", flag.ContinueOnError)

	oldArgs := os.Args
	os.Args = []string{"leech", "-version"}
	defer func() { os.Args = oldArgs }()

	app := NewCLIApplication()
	app.Out = io.Discard

	err := app.Run()
	if err != nil {
		t.Errorf("Run() with -version should return nil, got %v", err)
	}
}

func TestRunEmptyURL(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("leech", flag.ContinueOnError)

	oldArgs := os.Args
	os.Args = []string{"leech"}
	defer func() { os.Args = oldArgs }()

	app := NewCLIApplication()
	app.Out = io.Discard

	err := app.Run()
	if !errors.Is(err, errEmptyURL) {
		t.Errorf("Run() without URL should return errEmptyURL, got %v", err)
	}
}

func TestRunInvalidResource(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	flag.CommandLine = flag.NewFlagSet("leech", flag.ContinueOnError)

	oldArgs := os.Args
	os.Args = []string{"leech", ts.URL + "/file.bin"}
	defer func() { os.Args = oldArgs }()

	app := NewCLIApplication()
	app.Out = io.Discard
	app.Client = ts.Client()

	err := app.Run()
	if err == nil {
		t.Fatal("expected error for 404 resource")
	}
	if !strings.Contains(err.Error(), "no valid resources found") {
		t.Errorf("expected 'no valid resources found', got %v", err)
	}
}

func TestRunSuccessfulDownload(t *testing.T) {
	content := []byte("test download content")
	ts := newTestServer(content, false)
	defer ts.Close()

	dir := t.TempDir()

	flag.CommandLine = flag.NewFlagSet("leech", flag.ContinueOnError)

	oldArgs := os.Args
	os.Args = []string{"leech", "-output", dir, ts.URL + "/file.bin"}
	defer func() { os.Args = oldArgs }()

	app := NewCLIApplication()
	app.Out = io.Discard
	app.Client = ts.Client()

	err := app.Run()
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "file.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestRunMultipleFiles(t *testing.T) {
	content := []byte("multi file content")
	ts := newTestServer(content, false)
	defer ts.Close()

	dir := t.TempDir()

	flag.CommandLine = flag.NewFlagSet("leech", flag.ContinueOnError)

	oldArgs := os.Args
	os.Args = []string{"leech", "-output", dir, ts.URL + "/a.bin", ts.URL + "/b.bin"}
	defer func() { os.Args = oldArgs }()

	app := NewCLIApplication()
	app.Out = io.Discard
	app.Client = ts.Client()

	err := app.Run()
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"a.bin", "b.bin"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected file %s to exist", name)
		}
	}
}

func TestRunFailedDownload(t *testing.T) {
	// HEAD returns 200, GET returns 500
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "10")
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)

			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	dir := t.TempDir()

	flag.CommandLine = flag.NewFlagSet("leech", flag.ContinueOnError)

	oldArgs := os.Args
	os.Args = []string{"leech", "-output", dir, ts.URL + "/file.bin"}
	defer func() { os.Args = oldArgs }()

	app := NewCLIApplication()
	app.Out = io.Discard
	app.Client = ts.Client()

	err := app.Run()
	if err == nil {
		t.Fatal("expected error for failed download")
	}
	if !strings.Contains(err.Error(), "download(s) failed") {
		t.Errorf("expected 'download(s) failed', got %v", err)
	}
}
