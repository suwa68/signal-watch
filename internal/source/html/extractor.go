package html

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/suwa68/signal-watch/internal/source"
)

var ErrMissingTitle = errors.New("required title is missing")

// ItemError adds the zero-based item index and field to an extraction error.
type ItemError struct {
	Index int
	Field string
	Err   error
}

func (e *ItemError) Error() string {
	return fmt.Sprintf("extract item at index %d field %q: %v", e.Index, e.Field, e.Err)
}

func (e *ItemError) Unwrap() error {
	return e.Err
}

// Extractor turns an HTML document into normalized monitor items.
type Extractor interface {
	Extract(reader io.Reader, baseURL *url.URL, sourceID string, settings Settings) ([]source.MonitorItem, error)
}

// HTMLExtractor extracts repeated items from a static HTML document.
type HTMLExtractor struct{}

func NewExtractor() *HTMLExtractor {
	return &HTMLExtractor{}
}

func (e *HTMLExtractor) Extract(reader io.Reader, baseURL *url.URL, sourceID string, settings Settings) ([]source.MonitorItem, error) {
	document, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	matches := document.Find(settings.ItemSelector)
	items := make([]source.MonitorItem, 0, matches.Length())
	var extractionErr error

	matches.EachWithBreak(func(index int, item *goquery.Selection) bool {
		title := normalizedText(item.Find(settings.TitleSelector).First())
		if title == "" {
			extractionErr = &ItemError{Index: index, Field: "title", Err: ErrMissingTitle}
			return false
		}

		itemURL, err := extractItemURL(item, settings.URLSelector, baseURL)
		if err != nil {
			extractionErr = &ItemError{Index: index, Field: "url", Err: err}
			return false
		}

		content := ""
		if settings.ContentSelector != "" {
			content = normalizedText(item.Find(settings.ContentSelector).First())
		}

		items = append(items, source.MonitorItem{
			SourceID: sourceID,
			Title:    title,
			URL:      itemURL,
			Content:  content,
		})
		return true
	})

	if extractionErr != nil {
		return nil, extractionErr
	}
	return items, nil
}

func normalizedText(selection *goquery.Selection) string {
	if selection == nil || selection.Length() == 0 {
		return ""
	}
	return strings.Join(strings.Fields(selection.Text()), " ")
}

func extractItemURL(item *goquery.Selection, selector string, baseURL *url.URL) (string, error) {
	if selector == "" {
		return "", nil
	}

	link := item.Find(selector).First()
	href, ok := link.Attr("href")
	if !ok || strings.TrimSpace(href) == "" {
		return "", nil
	}

	reference, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", fmt.Errorf("parse href %q: %w", href, err)
	}
	if baseURL == nil {
		if reference.IsAbs() {
			return reference.String(), nil
		}
		return "", fmt.Errorf("resolve relative href %q without a base URL", href)
	}

	return baseURL.ResolveReference(reference).String(), nil
}
