package html

import (
	"context"
	"fmt"
	"net/http"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/source"
)

// Adapter composes document fetching and HTML extraction for html sources.
type Adapter struct {
	fetcher   Fetcher
	extractor Extractor
}

// New creates an HTML adapter backed by net/http.
func New(client *http.Client) *Adapter {
	return NewAdapter(NewHTTPFetcher(client), NewExtractor())
}

// NewAdapter creates an HTML adapter from independently testable components.
// Nil components receive the production defaults.
func NewAdapter(fetcher Fetcher, extractor Extractor) *Adapter {
	if fetcher == nil {
		fetcher = NewHTTPFetcher(nil)
	}
	if extractor == nil {
		extractor = NewExtractor()
	}
	return &Adapter{fetcher: fetcher, extractor: extractor}
}

func (a *Adapter) Collect(ctx context.Context, sourceConfig config.SourceConfig) ([]source.MonitorItem, error) {
	settings, err := ParseSettings(sourceConfig.Config)
	if err != nil {
		return nil, fmt.Errorf("parse html source %q settings: %w", sourceConfig.ID, err)
	}

	result, err := a.fetcher.Fetch(ctx, settings.URL)
	if err != nil {
		return nil, fmt.Errorf("fetch html source %q: %w", sourceConfig.ID, err)
	}
	if result == nil || result.Body == nil {
		return nil, fmt.Errorf("fetch html source %q: fetcher returned no response body", sourceConfig.ID)
	}
	defer result.Body.Close()

	items, err := a.extractor.Extract(result.Body, result.FinalURL, sourceConfig.ID, settings)
	if err != nil {
		return nil, fmt.Errorf("extract html source %q: %w", sourceConfig.ID, err)
	}
	return items, nil
}

var _ source.SourceAdapter = (*Adapter)(nil)
