package html

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// FetchResult contains the response stream and the final URL after redirects.
// The caller owns Body and must close it.
type FetchResult struct {
	Body     io.ReadCloser
	FinalURL *url.URL
}

// Fetcher retrieves an HTML document without coupling the adapter to net/http.
type Fetcher interface {
	Fetch(ctx context.Context, sourceURL string) (*FetchResult, error)
}

// HTTPFetcher retrieves public pages with HTTP GET requests.
type HTTPFetcher struct {
	client *http.Client
}

// NewHTTPFetcher creates a fetcher. A nil client uses http.DefaultClient.
func NewHTTPFetcher(client *http.Client) *HTTPFetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPFetcher{client: client}
}

// HTTPStatusError reports a non-successful HTTP response.
type HTTPStatusError struct {
	URL        string
	StatusCode int
	Status     string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("GET %s returned %s", e.URL, e.Status)
}

// Fetch performs one GET request. Redirect behavior follows the configured
// http.Client, and the returned final URL is taken from the final response.
func (f *HTTPFetcher) Fetch(ctx context.Context, sourceURL string) (*FetchResult, error) {
	parsedURL, err := parseHTTPURL(sourceURL)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create GET request: %w", err)
	}

	response, err := f.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", parsedURL, err)
	}

	finalURL := parsedURL
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.CopyN(io.Discard, response.Body, 4<<10)
		_ = response.Body.Close()
		return nil, &HTTPStatusError{
			URL:        finalURL.String(),
			StatusCode: response.StatusCode,
			Status:     response.Status,
		}
	}

	return &FetchResult{Body: response.Body, FinalURL: finalURL}, nil
}
