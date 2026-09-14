package html

import (
	"strings"
	"testing"
)

func TestParseSettings(t *testing.T) {
	t.Parallel()

	settings, err := ParseSettings(map[string]any{
		"url": "  HTTPS://example.com/news  ",
		"selectors": map[string]any{
			"item":    " article.news-item ",
			"title":   " h2.title ",
			"url":     " a.details ",
			"content": " .summary ",
			"future":  true,
		},
		"future": true,
	})
	if err != nil {
		t.Fatalf("ParseSettings() error = %v", err)
	}
	if settings.URL != "https://example.com/news" {
		t.Errorf("URL = %q, want https://example.com/news", settings.URL)
	}
	if settings.ItemSelector != "article.news-item" {
		t.Errorf("ItemSelector = %q", settings.ItemSelector)
	}
	if settings.TitleSelector != "h2.title" {
		t.Errorf("TitleSelector = %q", settings.TitleSelector)
	}
	if settings.URLSelector != "a.details" {
		t.Errorf("URLSelector = %q", settings.URLSelector)
	}
	if settings.ContentSelector != ".summary" {
		t.Errorf("ContentSelector = %q", settings.ContentSelector)
	}
}

func TestParseSettingsRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   map[string]any
		wantText string
	}{
		{name: "missing url", config: map[string]any{}, wantText: "config.url is required"},
		{name: "non-string url", config: map[string]any{"url": 10}, wantText: "config.url must be a string"},
		{name: "unsupported scheme", config: settingsConfig("ftp://example.com", "article", "h2"), wantText: "scheme must be http or https"},
		{name: "missing host", config: settingsConfig("https:///news", "article", "h2"), wantText: "host is required"},
		{name: "user info", config: settingsConfig("https://user@example.com/news", "article", "h2"), wantText: "user information is not supported"},
		{name: "missing selectors", config: map[string]any{"url": "https://example.com"}, wantText: "config.selectors is required"},
		{name: "selectors not object", config: map[string]any{"url": "https://example.com", "selectors": "article"}, wantText: "config.selectors: must be an object"},
		{name: "missing item selector", config: settingsConfig("https://example.com", "", "h2"), wantText: "config.selectors.item must not be empty"},
		{name: "missing title selector", config: settingsConfig("https://example.com", "article", ""), wantText: "config.selectors.title must not be empty"},
		{name: "optional selector wrong type", config: map[string]any{
			"url":       "https://example.com",
			"selectors": map[string]any{"item": "article", "title": "h2", "url": 3},
		}, wantText: "config.selectors.url must be a string"},
		{name: "invalid selector", config: settingsConfig("https://example.com", "article[", "h2"), wantText: "invalid CSS selector"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseSettings(test.config)
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("ParseSettings() error = %v, want text %q", err, test.wantText)
			}
		})
	}
}

func settingsConfig(sourceURL, itemSelector, titleSelector string) map[string]any {
	return map[string]any{
		"url": sourceURL,
		"selectors": map[string]any{
			"item":  itemSelector,
			"title": titleSelector,
		},
	}
}
