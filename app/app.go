package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var (
	errEmptyPipe         = errors.New("empty pipe")
	errEmptyURL          = errors.New("empty url")
	errInvalidURL        = errors.New("invalid url")
	errHTTPStatusIsNotOK = errors.New("http status is not ok")
)

const defaultChunkSize = 5

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

func isPiped() bool {
	fileInfo, _ := os.Stdin.Stat()
	return fileInfo.Mode()&os.ModeCharDevice == 0
}

func parseValidateURL(in string) (string, error) {
	u, err := url.ParseRequestURI(in)
	if err != nil {
		return "", fmt.Errorf("%s %w", errInvalidURL.Error(), err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errInvalidURL
	}
	return u.String(), nil
}

// NewCLIApplication creates new app instance.
func NewCLIApplication() *CLIApplication {
	flag.Usage = flagUsage(0, os.Stdin)

	optFlagVersion = flag.Bool("version", false, "display version information ("+Version+")")
	optFlagVerbose = flag.Bool("verbose", false, "verbose output")
	optFlagChunkSize = flag.Int("chunks", defaultChunkSize, "default chunk size")

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

func (c *CLIApplication) parseArgs() {
	if len(flag.Args()) > 0 {
		for _, arg := range flag.Args() {
			url, err := parseValidateURL(arg)
			if err == nil {
				c.URLS = append(c.URLS, url)
			}
		}
	}
}

func (c *CLIApplication) getChunks(length int, chunkSize int) [][2]int {
	out := [][2]int{}

	chunk := length / chunkSize

	start := 0
	end := 0
	for i := 0; i < chunkSize-1; i++ {
		start = i * (chunk + 1)
		end = start + chunk
		out = append(out, [2]int{start, end})
	}
	start = start + chunk + 1
	end = length - 1
	out = append(out, [2]int{start, end})
	return out
}

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

	r := &resource{
		url:    url,
		length: resp.ContentLength,
	}

	acceptRanges, ok := resp.Header["Accept-Ranges"]
	if ok {
		if resp.ContentLength > 0 && len(acceptRanges) > 0 && acceptRanges[0] == "bytes" {
			r.chunks = c.getChunks(int(resp.ContentLength), *optFlagChunkSize)
		}
	}

	contentType, ok := resp.Header["Content-Type"]
	if ok {
		r.contentType = contentType[0]
	}

	contentDisposition, ok := resp.Header["Content-Disposition"]
	if ok {
		_, params, err := mime.ParseMediaType(contentDisposition[0])
		if err == nil {
			r.filename = params["filename"]
		}
	}

	if r.filename == "" {
		basename := filepath.Base(url)
		r.filename = basename
		if r.contentType != "" {
			ext := findExtension(r.contentType)
			if ext != "unknown" {
				r.filename = basename + "." + ext
			}
		}
	}

	return r, nil
}

func findExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return "jpg"
	case "video/mp4":
		return "mp4"
	default:
		ext, err := mime.ExtensionsByType(mimeType)
		if err != nil {
			return "unknown"
		}
		return ext[0]
	}
}

// Run executes CLIApplication.
func (c *CLIApplication) Run() error {
	if *optFlagVersion {
		fmt.Fprintln(c.Out, Version)
		return nil
	}

	if isPiped() {
		if err := c.parsePipe(c.In); err != nil {
			return err
		}
	}
	c.parseArgs()

	if len(c.URLS) == 0 {
		return errEmptyURL
	}

	fmt.Println("optFlagVerbose", *optFlagVerbose)

	resource := make(chan *resource)

	for _, u := range c.URLS {
		go func(url string) {
			fmt.Println("firing ->", url)
			r, err := c.getResourceInformation(url)
			if err != nil {
				fmt.Println("-> err", err)
			}
			resource <- r
		}(u)
	}

	var downloadsCount int
	done := make(chan struct{})

	for range c.URLS {
		r := <-resource
		if r != nil {
			downloadsCount++
			go c.download(r, done)
		}
	}

	fmt.Println("downloadsCount", downloadsCount)

	for i := 0; i < downloadsCount; i++ {
		<-done
	}
	return nil
}

func (c *CLIApplication) download(r *resource, done chan struct{}) {
	fmt.Printf("%+v\n", r)
	if r.chunks != nil {
		var wg sync.WaitGroup

		fcontent := make([]byte, r.length)

		for i, chunkPair := range r.chunks {
			wg.Add(1)
			go func(part int, chunkPair [2]int) {
				defer wg.Done()
				byteParts, err := c.fetch(part, r.url, chunkPair)
				if err == nil {
					copy(fcontent[chunkPair[0]:], byteParts)
				}
			}(i, chunkPair)
		}
		wg.Wait()

		fmt.Printf("%+v\n", r)

		f, err := os.Create(r.filename)
		if err != nil {
			log.Fatal(err)
		}
		_, err = f.Write(fcontent)
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			_ = f.Close()
		}()

	}
	done <- struct{}{}
}

func (c *CLIApplication) fetch(part int, url string, chunk [2]int) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to Request (fetch), %w", err)
	}
	req.Header.Set("Range", "bytes="+strconv.Itoa(chunk[0])+"-"+strconv.Itoa(chunk[1]))

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to Do (fetch), %w", err)
	}
	fmt.Println("Status Code", resp.StatusCode)
	defer func() {
		_ = resp.Body.Close()
	}()

	fmt.Println("save part", part, "for url", url)
	// _, _ = io.Copy(ioutil.Discard, resp.Body)

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read all (fetch), %w", err)
	}

	return b, nil
}
