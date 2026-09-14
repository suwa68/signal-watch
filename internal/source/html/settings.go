package html

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/andybalholm/cascadia"
)

// Settings contains the HTML-specific portion of a source configuration.
type Settings struct {
	URL             string
	ItemSelector    string
	TitleSelector   string
	URLSelector     string
	ContentSelector string
}

// ParseSettings translates a generic source config block into validated HTML
// settings. Unknown fields are left untouched so the adapter config can evolve
// independently from the generic source schema.
func ParseSettings(raw map[string]any) (Settings, error) {
	rawURL, err := requiredString(raw, "url", "config")
	if err != nil {
		return Settings{}, err
	}

	parsedURL, err := parseHTTPURL(rawURL)
	if err != nil {
		return Settings{}, fmt.Errorf("config.url: %w", err)
	}

	selectorsValue, ok := raw["selectors"]
	if !ok {
		return Settings{}, fmt.Errorf("config.selectors is required")
	}
	selectors, err := asStringMap(selectorsValue)
	if err != nil {
		return Settings{}, fmt.Errorf("config.selectors: %w", err)
	}

	itemSelector, err := requiredString(selectors, "item", "config.selectors")
	if err != nil {
		return Settings{}, err
	}
	titleSelector, err := requiredString(selectors, "title", "config.selectors")
	if err != nil {
		return Settings{}, err
	}
	urlSelector, err := optionalString(selectors, "url", "config.selectors")
	if err != nil {
		return Settings{}, err
	}
	contentSelector, err := optionalString(selectors, "content", "config.selectors")
	if err != nil {
		return Settings{}, err
	}

	selectorFields := []struct {
		name  string
		value string
	}{
		{name: "item", value: itemSelector},
		{name: "title", value: titleSelector},
		{name: "url", value: urlSelector},
		{name: "content", value: contentSelector},
	}
	for _, field := range selectorFields {
		if field.value == "" {
			continue
		}
		if _, err := cascadia.Compile(field.value); err != nil {
			return Settings{}, fmt.Errorf("config.selectors.%s: invalid CSS selector %q: %w", field.name, field.value, err)
		}
	}

	return Settings{
		URL:             parsedURL.String(),
		ItemSelector:    itemSelector,
		TitleSelector:   titleSelector,
		URLSelector:     urlSelector,
		ContentSelector: contentSelector,
	}, nil
}

func parseHTTPURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("URL scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("URL host is required")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("URL user information is not supported")
	}

	return parsed, nil
}

func requiredString(values map[string]any, key, path string) (string, error) {
	value, ok := values[key]
	if !ok {
		return "", fmt.Errorf("%s.%s is required", path, key)
	}

	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s.%s must be a string", path, key)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("%s.%s must not be empty", path, key)
	}

	return text, nil
}

func optionalString(values map[string]any, key, path string) (string, error) {
	value, ok := values[key]
	if !ok || value == nil {
		return "", nil
	}

	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s.%s must be a string", path, key)
	}
	return strings.TrimSpace(text), nil
}

func asStringMap(value any) (map[string]any, error) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, nil
	case map[any]any:
		converted := make(map[string]any, len(typed))
		for key, entry := range typed {
			stringKey, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("must contain only string keys")
			}
			converted[stringKey] = entry
		}
		return converted, nil
	default:
		return nil, fmt.Errorf("must be an object")
	}
}
