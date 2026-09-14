package html

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPFetcherUsesGETAndReturnsFinalURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/start":
			http.Redirect(writer, request, "/final", http.StatusFound)
		case "/final":
			if request.Method != http.MethodGet {
				t.Errorf("request method = %s, want GET", request.Method)
			}
			_, _ = io.WriteString(writer, "<html>ok</html>")
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	result, err := NewHTTPFetcher(server.Client()).Fetch(context.Background(), server.URL+"/start")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	t.Cleanup(func() { _ = result.Body.Close() })
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != "<html>ok</html>" {
		t.Errorf("body = %q", body)
	}
	if got, want := result.FinalURL.String(), server.URL+"/final"; got != want {
		t.Errorf("FinalURL = %q, want %q", got, want)
	}
}

func TestHTTPFetcherRejectsNonSuccessfulStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	_, err := NewHTTPFetcher(server.Client()).Fetch(context.Background(), server.URL)
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("Fetch() error = %v, want HTTPStatusError", err)
	}
	if statusErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("StatusCode = %d, want %d", statusErr.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestHTTPFetcherPropagatesCancellation(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := NewHTTPFetcher(server.Client()).Fetch(ctx, server.URL)
		errCh <- err
	}()

	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach test server")
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Fetch() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fetch() did not return after cancellation")
	}
}

func TestHTTPFetcherRejectsUnsupportedURLs(t *testing.T) {
	t.Parallel()

	for _, sourceURL := range []string{"file:///tmp/news.html", "https:///missing-host", "https://user@example.com/news"} {
		sourceURL := sourceURL
		t.Run(sourceURL, func(t *testing.T) {
			t.Parallel()
			_, err := NewHTTPFetcher(nil).Fetch(context.Background(), sourceURL)
			if err == nil || !strings.Contains(err.Error(), "URL") {
				t.Fatalf("Fetch() error = %v, want URL validation error", err)
			}
		})
	}
}
