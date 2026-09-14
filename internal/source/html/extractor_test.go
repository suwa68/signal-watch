package html

import (
	"errors"
	"io"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/suwa68/signal-watch/internal/source"
)

func TestHTMLExtractorExtractsFixture(t *testing.T) {
	t.Parallel()

	fixture, err := os.Open("testdata/news.html")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = fixture.Close() })
	baseURL, err := url.Parse("https://example.com/news/index.html")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	got, err := NewExtractor().Extract(fixture, baseURL, "news", Settings{
		ItemSelector:    "article.news-item",
		TitleSelector:   ".title",
		URLSelector:     "a.details",
		ContentSelector: ".summary",
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	want := []source.MonitorItem{
		{
			SourceID: "news",
			Title:    "First release",
			URL:      "https://example.com/detail/first?lang=en#overview",
			Content:  "Alpha launches today.",
		},
		{
			SourceID: "news",
			Title:    "Second release",
			URL:      "https://other.example/news/second",
			Content:  "Beta is available.",
		},
		{
			SourceID: "news",
			Title:    "Third release",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Extract() = %#v, want %#v", got, want)
	}
}

func TestHTMLExtractorReturnsEmptySliceWhenNothingMatches(t *testing.T) {
	t.Parallel()

	items, err := NewExtractor().Extract(strings.NewReader("<html><body></body></html>"), nil, "news", Settings{
		ItemSelector:  "article",
		TitleSelector: "h2",
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("Extract() = %#v, want non-nil empty slice", items)
	}
}

func TestHTMLExtractorReportsMissingTitleWithItemIndex(t *testing.T) {
	t.Parallel()

	_, err := NewExtractor().Extract(strings.NewReader(`
<article><h2>First</h2></article>
<article><h2>  </h2></article>
`), nil, "news", Settings{ItemSelector: "article", TitleSelector: "h2"})
	if !errors.Is(err, ErrMissingTitle) {
		t.Fatalf("Extract() error = %v, want ErrMissingTitle", err)
	}
	var itemErr *ItemError
	if !errors.As(err, &itemErr) || itemErr.Index != 1 || itemErr.Field != "title" {
		t.Fatalf("Extract() error = %#v, want title ItemError at index 1", err)
	}
}

func TestHTMLExtractorReportsInvalidHref(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("https://example.com/news")
	_, err := NewExtractor().Extract(strings.NewReader(`<article><h2>Title</h2><a href="%zz">Link</a></article>`), baseURL, "news", Settings{
		ItemSelector:  "article",
		TitleSelector: "h2",
		URLSelector:   "a",
	})
	var itemErr *ItemError
	if !errors.As(err, &itemErr) || itemErr.Field != "url" {
		t.Fatalf("Extract() error = %v, want URL ItemError", err)
	}
}

func TestHTMLExtractorRejectsRelativeHrefWithoutBaseURL(t *testing.T) {
	t.Parallel()

	_, err := NewExtractor().Extract(strings.NewReader(`<article><h2>Title</h2><a href="detail">Link</a></article>`), nil, "news", Settings{
		ItemSelector:  "article",
		TitleSelector: "h2",
		URLSelector:   "a",
	})
	if err == nil || !strings.Contains(err.Error(), "without a base URL") {
		t.Fatalf("Extract() error = %v, want missing base URL error", err)
	}
}

func TestHTMLExtractorPropagatesReaderError(t *testing.T) {
	t.Parallel()

	_, err := NewExtractor().Extract(errorReader{}, nil, "news", Settings{ItemSelector: "article", TitleSelector: "h2"})
	if err == nil || !strings.Contains(err.Error(), "parse HTML") {
		t.Fatalf("Extract() error = %v, want parse HTML error", err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
