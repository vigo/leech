package app

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

var (
	errEmptyPipe  = errors.New("empty pipe")
	errEmptyURL   = errors.New("empty url")
	errInvalidURL = errors.New("invalid url")
)

// CLIApplication represents new app instance.
type CLIApplication struct {
	In   io.Reader
	Out  io.Writer
	URLS []string
}

func flagUsage(code int, out io.Writer) func() {
	return func() {
		fmt.Fprintf(
			out,
			cmdUsage,
			os.Args[0],
			Version,
		)
		if code > 0 {
			os.Exit(code)
		}
	}
}

// NewCLIApplication creates new app instance.
func NewCLIApplication() *CLIApplication {
	flag.Usage = flagUsage(0, os.Stdin)

	optFlagVersion = flag.Bool("version", false, "display version information ("+Version+")")
	optFlagVerbose = flag.Bool("verbose", false, "verbose output")

	flag.Parse()

	return &CLIApplication{
		In:  os.Stdin,
		Out: os.Stdout,
	}
}

func (c *CLIApplication) isPiped() bool {
	fileInfo, _ := os.Stdin.Stat()
	return fileInfo.Mode()&os.ModeCharDevice == 0
}

func (c *CLIApplication) parseValidateURL(in string) (string, error) {
	u, err := url.ParseRequestURI(in)
	if err != nil {
		return "", fmt.Errorf("%s %w", errInvalidURL.Error(), err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errInvalidURL
	}
	return u.String(), nil
}

func (c *CLIApplication) parsePipe(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		url, err := c.parseValidateURL(scanner.Text())
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
	if len(flag.Args()) > 0 {
		for _, arg := range flag.Args() {
			url, err := c.parseValidateURL(arg)
			if err == nil {
				c.URLS = append(c.URLS, url)
			}
		}
	}
}

// Run executes CLIApplication.
func (c *CLIApplication) Run() error {
	if *optFlagVersion {
		fmt.Fprintln(c.Out, Version)
		return nil
	}

	if c.isPiped() {
		if err := c.parsePipe(c.In); err != nil {
			return err
		}
	}
	c.parseArgs()

	if len(c.URLS) == 0 {
		return errEmptyURL
	}
	if *optFlagVerbose {
		fmt.Fprintf(c.Out, "will download %d file(s)\n%s\n", len(c.URLS), strings.Join(c.URLS, "\n"))
	}
	return nil
}
