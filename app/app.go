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

const (
	defaultChunkSize = 5
	permDir          = 0o750
	permFile         = 0o600
)

// CLIApplication represents the download manager instance.
type CLIApplication struct {
	In        io.Reader
	Out       io.Writer
	URLS      []string
	Client    *http.Client
	chunkSize int
	outputDir string
	verbose   bool
	limiter   *rateLimiter
}

// NewCLIApplication creates and configures a new CLI app instance.
func NewCLIApplication() *CLIApplication {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true

	return &CLIApplication{
		In:     os.Stdin,
		Out:    os.Stdout,
		Client: &http.Client{Transport: transport},
	}
}

var errVersionRequested = errors.New("")

func (c *CLIApplication) parseFlags() error {
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
		fmt.Fprintln(c.Out, Version)

		return errVersionRequested
	}

	rate, err := parseRate(flagLimit)
	if err != nil {
		return fmt.Errorf("invalid limit: %w", err)
	}

	c.chunkSize = flagChunkSize
	c.outputDir = flagOutput
	c.verbose = flagVerbose
	c.limiter = newRateLimiter(rate)

	return nil
}

func (c *CLIApplication) setupLogging() {
	level := slog.LevelWarn
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

func (c *CLIApplication) parseArgs(args []string) {
	for _, arg := range args {
		url, err := parseValidateURL(arg)
		if err == nil {
			c.URLS = append(c.URLS, url)
		}
	}
}

// Run executes the download manager.
func (c *CLIApplication) Run() error {
	if err := c.parseFlags(); err != nil {
		if errors.Is(err, errVersionRequested) {
			return nil
		}

		return err
	}

	c.setupLogging()

	if isPiped() {
		if err := c.parsePipe(c.In); err != nil {
			return err
		}
	}
	c.parseArgs(flag.Args())

	if len(c.URLS) == 0 {
		return errEmptyURL
	}

	if err := os.MkdirAll(c.outputDir, permDir); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// phase 1: collect resource info for all URLs (HEAD requests)
	slog.Info("checking resources", "urls", len(c.URLS))

	resChan := make(chan *resource)

	for _, u := range c.URLS {
		go func(url string) {
			slog.Debug("fetching resource info", logKeyURL, url)
			r, err := c.getResourceInformation(url)
			if err != nil {
				slog.Error("resource info failed", logKeyURL, url, logKeyError, err)
				resChan <- nil

				return
			}
			resChan <- r
		}(u)
	}

	var resources []*resource

	var totalSize int64

	for range c.URLS {
		r := <-resChan
		if r != nil {
			resources = append(resources, r)
			if r.length > 0 {
				totalSize += r.length
			}
		}
	}

	if len(resources) == 0 {
		return errors.New("no valid resources found")
	}

	// phase 2: show summary and check disk space
	slog.Info("download summary",
		"files", len(resources),
		"total_size", formatBytes(totalSize),
		"output", c.outputDir,
	)

	if totalSize > 0 {
		if err := checkDiskSpace(c.outputDir, totalSize); err != nil {
			return err
		}
	}

	// phase 3: start downloads
	slog.Info("starting downloads", "files", len(resources), "chunks", c.chunkSize)

	pd := newProgressDisplay()
	done := make(chan struct{})

	for _, r := range resources {
		go c.download(r, done, pd)
	}

	pd.start()

	for i := range resources {
		<-done

		// check disk space after each download completes
		remaining := totalSize
		for _, r := range resources[:i+1] {
			remaining -= r.length
		}

		if remaining > 0 {
			if err := checkDiskSpace(c.outputDir, remaining); err != nil {
				slog.Error("disk space warning", logKeyError, err)
			}
		}
	}

	pd.finish()

	slog.Info("all downloads complete", "count", len(resources))

	return nil
}
