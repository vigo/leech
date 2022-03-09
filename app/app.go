package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
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

// NewCLIApplication creates new app instance.
func NewCLIApplication() *CLIApplication {
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
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			url, err := c.parseValidateURL(arg)
			if err == nil {
				c.URLS = append(c.URLS, url)
			}
		}
	}
}

// Run executes CLIApplication.
func (c *CLIApplication) Run() error {
	if c.isPiped() {
		if err := c.parsePipe(c.In); err != nil {
			return err
		}
	}
	c.parseArgs()

	if len(c.URLS) == 0 {
		return errEmptyURL
	}
	return nil
}
