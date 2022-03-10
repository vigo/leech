package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	errEmptyPipe         = errors.New("empty pipe")
	errEmptyURL          = errors.New("empty url")
	errInvalidURL        = errors.New("invalid url")
	errHTTPStatusIsNotOK = errors.New("http status is not ok")
)

// CLIApplication represents new app instance.
type CLIApplication struct {
	In     io.Reader
	Out    io.Writer
	URLS   []string
	Client *http.Client
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

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	client := &http.Client{
		Transport: transport,
	}

	return &CLIApplication{
		In:     os.Stdin,
		Out:    os.Stdout,
		Client: client,
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

// func (c *CLIApplication) getChunks(length int, chunkSize int) [][2]int {
// 	out := [][2]int{}
//
// 	chunk := length / chunkSize
//
// 	start := 0
// 	end := 0
// 	for i := 0; i < chunkSize-1; i++ {
// 		start = i * (chunk + 1)
// 		end = start + chunk
// 		out = append(out, [2]int{start, end})
// 	}
// 	start = start + chunk + 1
// 	end = length - 1
// 	out = append(out, [2]int{start, end})
// 	return out
// }

type contentInformation struct {
	acceptRanges  string
	contentType   string
	filename      string
	contentLength int64
}

func (c *CLIApplication) getContentInformation(url string) (*contentInformation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to Request, %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to Do request, %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w, returned %d", errHTTPStatusIsNotOK, resp.StatusCode)
	}

	contentInfo := &contentInformation{}

	acceptRanges, ok := resp.Header["Accept-Ranges"]
	if ok {
		contentInfo.acceptRanges = acceptRanges[0]
	}

	contentType, ok := resp.Header["Content-Type"]
	if ok {
		contentInfo.contentType = contentType[0]
	}

	contentInfo.contentLength = resp.ContentLength

	contentDisposition, ok := resp.Header["Content-Disposition"]
	if ok {
		_, params, err := mime.ParseMediaType(contentDisposition[0])
		if err == nil {
			contentInfo.filename = params["filename"]
		}
	}

	fmt.Printf("%+v\n", resp)
	return contentInfo, nil
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

	info, err := c.getContentInformation(c.URLS[0])
	if err != nil {
		return err
	}
	fmt.Printf("%+v\n", info)
	return nil
}
