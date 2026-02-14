package app

import (
	"strings"
	"testing"
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
