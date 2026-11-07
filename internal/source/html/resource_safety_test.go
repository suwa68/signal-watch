package html

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/suwa68/signal-watch/internal/config"
)

// These boundaries catch accepting a valid HTML prefix after the response limit,
// reading the entire oversized stream, or leaking a response body on failure.
func TestResourceSafetyResponseBytes(t *testing.T) {
	t.Parallel()
	const limit = 4 << 20
	for _, test := range []struct {
		name      string
		size      int
		length    int64
		wantError bool
	}{
		{"below limit", limit - 1, int64(limit - 1), false},
		{"exact limit", limit, int64(limit), false},
		{"known oversized", limit + 4096, int64(limit + 4096), true},
		{"unknown exact limit", limit, -1, false},
		{"unknown oversized", limit + 4096, -1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &safetyBody{reader: strings.NewReader(safetyDocument(test.size))}
			client := &http.Client{Transport: safetyTransport(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Status: "200 OK", Body: body, ContentLength: test.length, Header: make(http.Header), Request: request}, nil
			})}
			items, err := New(client).Collect(context.Background(), safetyConfig("https://example.test/news"))
			if !body.closed {
				t.Error("Collect did not close the response body")
			}
			if body.read > limit+1 {
				t.Errorf("read %d bytes, want at most %d", body.read, limit+1)
			}
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "limit") {
					t.Errorf("Collect error = %v, want response limit error", err)
				}
				if items != nil {
					t.Errorf("Collect returned %d partial items on overflow", len(items))
				}
			} else if err != nil || len(items) != 1 || items[0].Title != "Release" {
				t.Errorf("Collect = %#v, %v; want one Release item", items, err)
			}
		})
	}
}

func TestResourceSafetyStreamingResponses(t *testing.T) {
	t.Parallel()
	for _, encoding := range []string{"chunked", "gzip"} {
		t.Run(encoding, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if encoding == "gzip" {
					writer.Header().Set("Content-Encoding", "gzip")
					compressed := gzip.NewWriter(writer)
					defer compressed.Close()
					_, _ = io.WriteString(compressed, safetyDocument((4<<20)+1))
					return
				}
				writer.(http.Flusher).Flush() // Force unknown-length chunked transport.
				_, _ = io.WriteString(writer, safetyDocument((4<<20)+1))
			}))
			defer server.Close()
			items, err := New(server.Client()).Collect(context.Background(), safetyConfig(server.URL))
			if err == nil || !strings.Contains(err.Error(), "limit") || items != nil {
				t.Fatalf("Collect = %d items, %v; want no items and limit error", len(items), err)
			}
		})
	}
}

func TestResourceSafetyReadFailureClosesBody(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("response stream failed")
	body := &safetyBody{reader: io.MultiReader(strings.NewReader("<article><h2>Release</h2></article>"), safetyErrorReader{wantErr})}
	adapter := NewAdapter(fetcherFunc(func(context.Context, string) (*FetchResult, error) {
		return &FetchResult{Body: body}, nil
	}), nil)
	items, err := adapter.Collect(context.Background(), safetyConfig("https://example.test"))
	if !errors.Is(err, wantErr) || items != nil {
		t.Errorf("Collect = %#v, %v; want no items and underlying read error", items, err)
	}
	if !body.closed {
		t.Error("read failure did not close response body")
	}
}

func TestResourceSafetyStatusFailureClosesBody(t *testing.T) {
	t.Parallel()
	body := &safetyBody{reader: strings.NewReader("unavailable")}
	client := &http.Client{Transport: safetyTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Status: "503 Service Unavailable", Body: body, Header: make(http.Header), Request: request}, nil
	})}
	items, err := New(client).Collect(context.Background(), safetyConfig("https://example.test"))
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || items != nil {
		t.Errorf("Collect = %#v, %v; want status error without items", items, err)
	}
	if !body.closed {
		t.Error("HTTP status failure did not close response body")
	}
	if body.read != 0 {
		t.Errorf("read %d bytes from rejected status body, want immediate close without draining", body.read)
	}
}

func TestResourceSafetyCancellationDuringRead(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	body := &safetyBody{reader: safetyBlockingReader{ctx, started}}
	adapter := NewAdapter(fetcherFunc(func(context.Context, string) (*FetchResult, error) {
		return &FetchResult{Body: body}, nil
	}), nil)
	done := make(chan error, 1)
	go func() {
		items, err := adapter.Collect(ctx, safetyConfig("https://example.test"))
		if items != nil {
			err = fmt.Errorf("returned partial items: %v", items)
		}
		done <- err
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("collection never began reading")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Collect error = %v, want context.Canceled", err)
		}
		if !body.closed {
			t.Error("cancellation did not close response body")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("collection did not return after cancellation")
	}
}

func TestResourceSafetyCanceledCollectionCannotReturnItems(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &safetyBody{reader: strings.NewReader(safetyDocument(64))}
	adapter := NewAdapter(fetcherFunc(func(context.Context, string) (*FetchResult, error) {
		cancel() // A response can arrive at the same time as caller cancellation.
		return &FetchResult{Body: body}, nil
	}), nil)
	items, err := adapter.Collect(ctx, safetyConfig("https://example.test"))
	if !errors.Is(err, context.Canceled) || items != nil {
		t.Errorf("Collect = %#v, %v; want no items and context.Canceled", items, err)
	}
	if !body.closed {
		t.Error("canceled successful fetch did not close its response body")
	}
}

func TestResourceSafetyCollectionDeadline(t *testing.T) {
	t.Parallel()
	for _, callerDeadline := range []bool{false, true} {
		t.Run(fmt.Sprint(callerDeadline), func(t *testing.T) {
			ctx := context.Background()
			if callerDeadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Second)
				defer cancel()
			}
			var fetchCtx context.Context
			adapter := NewAdapter(fetcherFunc(func(fetchContext context.Context, _ string) (*FetchResult, error) {
				fetchCtx = fetchContext
				deadline, ok := fetchContext.Deadline()
				if !ok || time.Until(deadline) > 15*time.Second {
					t.Error("fetch context lacks bounded collection deadline")
				}
				if want, ok := ctx.Deadline(); ok && !deadline.Equal(want) {
					t.Error("collection extended caller deadline")
				}
				return &FetchResult{Body: io.NopCloser(strings.NewReader(safetyDocument(64)))}, nil
			}), nil)
			_, err := adapter.Collect(ctx, safetyConfig("https://example.test"))
			if err != nil {
				t.Fatal(err)
			}
			if fetchCtx.Err() != context.Canceled {
				t.Errorf("fetch context after return = %v, want canceled to release timer", fetchCtx.Err())
			}
		})
	}
}

func TestResourceSafetyItemLimit(t *testing.T) {
	t.Parallel()
	for _, count := range []int{1000, 1001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			adapter := NewAdapter(fetcherFunc(func(context.Context, string) (*FetchResult, error) {
				return &FetchResult{Body: io.NopCloser(strings.NewReader(strings.Repeat("<article><h2>Release</h2></article>", count)))}, nil
			}), nil)
			items, err := adapter.Collect(context.Background(), safetyConfig("https://example.test"))
			if count == 1000 {
				if err != nil || len(items) != 1000 {
					t.Fatalf("Collect = %d items, %v; want exactly 1000 items", len(items), err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "limit") || items != nil {
				t.Fatalf("Collect = %d items, %v; want no items and item limit error", len(items), err)
			}
		})
	}
}

func safetyConfig(sourceURL string) config.SourceConfig {
	return config.SourceConfig{ID: "safety", Type: "html", Config: settingsConfig(sourceURL, "article", "h2")}
}

func safetyDocument(size int) string {
	const article = "<article><h2>Release</h2></article>"
	return article + strings.Repeat(" ", size-len(article))
}

type safetyTransport func(*http.Request) (*http.Response, error)

func (f safetyTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type safetyBody struct {
	reader io.Reader
	read   int
	closed bool
}

func (b *safetyBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += n
	return n, err
}
func (b *safetyBody) Close() error { b.closed = true; return nil }

type safetyErrorReader struct{ err error }

func (r safetyErrorReader) Read([]byte) (int, error) { return 0, r.err }

type safetyBlockingReader struct {
	ctx     context.Context
	started chan struct{}
}

func (r safetyBlockingReader) Read([]byte) (int, error) {
	close(r.started)
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}
