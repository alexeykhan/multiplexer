package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type (
	Crawler interface {
		Crawl(ctx context.Context, urls []string) ([]Result, error)
	}
	Result struct {
		SourceURL    string
		StatusCode   int
		ResponseBody json.RawMessage

		err error
	}
	Config struct {
		MaxConnections uint16        // Number of simultaneous requests.
		RequestTimeout time.Duration // Timeout per request.
	}
	crawler struct {
		config Config       // Crawler settings.
		client *http.Client // Reusable HTTP-client for outgoing requests.
	}
)

var (
	// Interface compliance check.
	_ Crawler = (*crawler)(nil)

	// defaultConfig stores predefined settings.
	defaultConfig = Config{
		MaxConnections: 4,
		RequestTimeout: time.Second,
	}
)

// New returns a new instance of Crawler with default settings.
func New() Crawler {
	c, _ := NewWithConfig(defaultConfig)
	return c
}

// NewWithConfig returns a new instance of Crawler with custom settings.
func NewWithConfig(cfg Config) (Crawler, error) {
	maxConnections := int(cfg.MaxConnections)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = maxConnections
	tr.MaxConnsPerHost = maxConnections
	tr.MaxIdleConnsPerHost = maxConnections

	return &crawler{
		config: cfg,
		client: &http.Client{
			Timeout:   cfg.RequestTimeout,
			Transport: tr,
		},
	}, nil
}

// Crawl loops through the given URLs list, tries to get a response from
// each and return either a slice of results, or the first error if present.
func (cr *crawler) Crawl(ctx context.Context, urls []string) ([]Result, error) {
	select {
	case <-ctx.Done():
		log.Println("crawler: exit on context done:", ctx.Err())
		return nil, ctx.Err()
	default:
	}

	if len(urls) == 0 {
		return nil, nil
	}

	log.Printf("crawler: received %d tasks: validating URL format\n", len(urls))

	var invalidURLErr error
	tasks := make(chan string, len(urls))
	for _, checkURL := range urls {
		// Check general cases for invalid URLs.
		// Unfortunately, cases like "http://invalidurl" successfully pass this check.
		if uri, err := url.ParseRequestURI(checkURL); err != nil || uri.Host == "" || uri.Scheme == "" {
			log.Println("crawler: invalid url:", checkURL)
			invalidURLErr = fmt.Errorf("invalid url: %q", checkURL)
			break
		}
		tasks <- checkURL
	}
	close(tasks)

	if invalidURLErr != nil {
		return nil, invalidURLErr
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Given condition: limit the number of outgoing requests.
	numWorkers := int(cr.config.MaxConnections)
	if numWorkers > len(urls) {
		numWorkers = len(urls)
	}

	results := make(chan Result)
	wg := &sync.WaitGroup{}
	wg.Add(numWorkers)

	log.Printf("crawler: starting %d workers\n", numWorkers)
	for i := 0; i < numWorkers; i++ {
		go cr.worker(ctx, wg, tasks, results)
	}

	go func() {
		wg.Wait()
		close(results)
		log.Println("crawler: results channel closed")
	}()

	var exitErr error
	out := make([]Result, 0, len(urls))
	for res := range results {
		if exitErr != nil {
			log.Println("crawler: error occurred: skipping new results")
			continue
		}
		if res.err != nil {
			log.Println("crawler: error occurred: stopping other goroutines")
			exitErr = fmt.Errorf("failed to crawl %q: %w", res.SourceURL, res.err)
			cancel()
			continue
		}
		log.Println("crawler: received new result")
		out = append(out, res)
	}

	if exitErr != nil {
		log.Println("crawler: exit with error:", exitErr)
		return nil, exitErr
	}

	log.Println("crawler: all tasks done")
	return out, nil
}

// worker reads tasks from the queue and calls crawl to do the job for it.
func (cr *crawler) worker(ctx context.Context, wg *sync.WaitGroup, tasks chan string, results chan Result) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			log.Println("crawler: worker stopped:", ctx.Err())
			return
		case url, open := <-tasks:
			if !open {
				log.Println("crawler: worker stopped: no more tasks")
				return
			}
			results <- cr.crawl(ctx, url)
		}
	}
}

// crawl does all the job: send a request, receives a response and passes it back to caller.
func (cr *crawler) crawl(ctx context.Context, url string) (res Result) {
	res = Result{SourceURL: url}

	select {
	case <-ctx.Done():
		log.Printf("crawler: crawl stopped before starting: %s -> %s\n", url, ctx.Err())
		res.err = fmt.Errorf("exit on context done: %w", ctx.Err())
		return
	default:
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Printf("crawler: create get request for %s: %s", url, err.Error())
		res.err = fmt.Errorf("create a request: %w", err)
		return
	}

	// NOTE: Uncomment to see that code really blocks on N concurrent requests.
	// time.Sleep(5 * time.Second)

	req = req.WithContext(ctx)
	log.Println("crawler: sending request:", url)

	resp, err := cr.client.Do(req)
	if err != nil {
		log.Println("crawler: send request:", err)
		res.err = fmt.Errorf("failed to send a request: %w", err)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Println("crawler: close response body:", err)
		}
	}()

	// Check response status code.
	if resp.StatusCode != http.StatusOK {
		log.Printf("crawler: request failed: %s: status: %d", res.SourceURL, resp.StatusCode)
		res.err = fmt.Errorf("unexpected response status code: %d", resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("crawler: read response body:", err)
		res.err = fmt.Errorf("read a response body: %w", err)
		return
	}

	// Check if response body is a valid JSON.
	var js interface{}
	if err := json.Unmarshal(body, &js); err != nil {
		log.Println("crawler: unmarshal response body to JSON:", err)
		res.err = fmt.Errorf("unmarshal response body to JSON: %w", err)
		return
	}

	// Remove all special characters from body.
	buffer := new(bytes.Buffer)
	if err := json.Compact(buffer, body); err != nil {
		log.Println("crawler: compact JSON to buffer:", err)
		res.err = fmt.Errorf("compact JSON to buffer: %w", err)
		return
	}

	log.Printf("crawler: task finished: %s [%d]\n", url, resp.StatusCode)

	return Result{
		SourceURL:    url,
		StatusCode:   resp.StatusCode,
		ResponseBody: json.RawMessage(buffer.String()),
	}
}
