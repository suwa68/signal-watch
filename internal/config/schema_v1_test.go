package config

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseSourceConfigV1(t *testing.T) {
	t.Parallel()

	decoded := validDecodedConfig()
	got, err := ParseSourceConfig(decoded)
	if err != nil {
		t.Fatalf("ParseSourceConfig() error = %v", err)
	}
	want := SourceConfig{
		ID:              "example",
		Name:            "Example Source",
		Type:            "html",
		IntervalSeconds: 300,
		Config: map[string]any{
			"url":    "https://example.com/news",
			"future": true,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseSourceConfig() = %#v, want %#v", got, want)
	}

	got.Config["url"] = "changed"
	if decoded["config"].(map[string]any)["url"] == "changed" {
		t.Fatal("ParseSourceConfig() returned the input config map without copying it")
	}
}

func TestParseSourceConfigRejectsInvalidDefinitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(DecodedConfig)
		wantIs   error
		wantText string
	}{
		{
			name:   "missing version",
			mutate: func(config DecodedConfig) { delete(config, "version") },
			wantIs: ErrConfigVersionRequired,
		},
		{
			name:     "version has wrong type",
			mutate:   func(config DecodedConfig) { config["version"] = "1" },
			wantText: "version must be an integer",
		},
		{
			name:   "unsupported version",
			mutate: func(config DecodedConfig) { config["version"] = 2 },
			wantIs: ErrUnsupportedConfigVersion,
		},
		{
			name:     "unknown top-level field",
			mutate:   func(config DecodedConfig) { config["selector"] = ".item" },
			wantText: "unknown top-level field(s): selector",
		},
		{
			name:     "missing id",
			mutate:   func(config DecodedConfig) { delete(config, "id") },
			wantText: `field "id" is required`,
		},
		{
			name:     "empty name",
			mutate:   func(config DecodedConfig) { config["name"] = "  " },
			wantText: `field "name" must not be empty`,
		},
		{
			name:     "type has wrong type",
			mutate:   func(config DecodedConfig) { config["type"] = 10 },
			wantText: `field "type" must be a string`,
		},
		{
			name:     "missing interval",
			mutate:   func(config DecodedConfig) { delete(config, "interval_seconds") },
			wantText: `field "interval_seconds" is required`,
		},
		{
			name:     "interval has wrong type",
			mutate:   func(config DecodedConfig) { config["interval_seconds"] = 1.5 },
			wantText: `field "interval_seconds" must be an integer`,
		},
		{
			name:     "non-positive interval",
			mutate:   func(config DecodedConfig) { config["interval_seconds"] = 0 },
			wantText: `field "interval_seconds" must be greater than zero`,
		},
		{
			name:     "missing adapter config",
			mutate:   func(config DecodedConfig) { delete(config, "config") },
			wantText: `field "config" is required`,
		},
		{
			name:     "adapter config is not an object",
			mutate:   func(config DecodedConfig) { config["config"] = "html" },
			wantText: `field "config" must be an object`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decoded := validDecodedConfig()
			test.mutate(decoded)
			_, err := ParseSourceConfig(decoded)
			if err == nil {
				t.Fatal("ParseSourceConfig() error = nil")
			}
			if test.wantIs != nil && !errors.Is(err, test.wantIs) {
				t.Fatalf("ParseSourceConfig() error = %v, want errors.Is(%v)", err, test.wantIs)
			}
			if test.wantText != "" && !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("ParseSourceConfig() error = %v, want text %q", err, test.wantText)
			}
		})
	}
}

func TestYAMLDecoderAndSchemaV1Compose(t *testing.T) {
	t.Parallel()

	decoded, err := NewYAMLConfigDecoder().Decode(SourceDefinitionDocument{Content: []byte(`
version: 1
id: example
name: Example Source
type: html
interval_seconds: 60
config:
  url: https://example.com
  selectors:
    item: article
    title: h2
`)})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	parsed, err := ParseSourceConfig(decoded)
	if err != nil {
		t.Fatalf("ParseSourceConfig() error = %v", err)
	}
	if parsed.ID != "example" || parsed.Type != "html" || parsed.IntervalSeconds != 60 {
		t.Fatalf("ParseSourceConfig() = %#v", parsed)
	}
	if _, ok := parsed.Config["selectors"]; !ok {
		t.Fatal("ParseSourceConfig() removed adapter-specific selectors")
	}
}

func validDecodedConfig() DecodedConfig {
	return DecodedConfig{
		"version":          1,
		"id":               "example",
		"name":             "Example Source",
		"type":             "html",
		"interval_seconds": 300,
		"config": map[string]any{
			"url":    "https://example.com/news",
			"future": true,
		},
	}
}
