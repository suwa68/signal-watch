package html

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/source"
)

func TestAdapterCollectsHTMLItems(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `<article><h2>Release</h2><a href="/release">Details</a><p>Available now</p></article>`)
	}))
	t.Cleanup(server.Close)

	items, err := New(server.Client()).Collect(context.Background(), config.SourceConfig{
		ID:   "news",
		Type: "html",
		Config: map[string]any{
			"url": server.URL + "/news",
			"selectors": map[string]any{
				"item":    "article",
				"title":   "h2",
				"url":     "a",
				"content": "p",
			},
		},
	})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	want := []source.MonitorItem{{
		SourceID: "news",
		Title:    "Release",
		URL:      server.URL + "/release",
		Content:  "Available now",
	}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("Collect() = %#v, want %#v", items, want)
	}
}

func TestAdapterDoesNotFetchInvalidSettings(t *testing.T) {
	t.Parallel()

	called := false
	adapter := NewAdapter(fetcherFunc(func(context.Context, string) (*FetchResult, error) {
		called = true
		return nil, nil
	}), nil)
	_, err := adapter.Collect(context.Background(), config.SourceConfig{ID: "broken", Config: map[string]any{}})
	if err == nil {
		t.Fatal("Collect() error = nil")
	}
	if called {
		t.Fatal("Collect() called fetcher for invalid settings")
	}
}

func TestAdapterPropagatesComponentErrors(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("fetch failed")
	adapter := NewAdapter(fetcherFunc(func(context.Context, string) (*FetchResult, error) {
		return nil, wantErr
	}), nil)
	_, err := adapter.Collect(context.Background(), config.SourceConfig{
		ID:     "news",
		Config: settingsConfig("https://example.com", "article", "h2"),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Collect() error = %v, want fetch error", err)
	}
}

type fetcherFunc func(context.Context, string) (*FetchResult, error)

func (f fetcherFunc) Fetch(ctx context.Context, sourceURL string) (*FetchResult, error) {
	return f(ctx, sourceURL)
}
