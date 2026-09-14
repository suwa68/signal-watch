package config

import (
	"errors"
	"strings"
	"testing"
)

func TestYAMLConfigDecoderDecodesOneDocument(t *testing.T) {
	t.Parallel()

	document := SourceDefinitionDocument{
		Key:     "example.yaml",
		Content: []byte("version: 1\nid: example\nconfig:\n  enabled: true\n"),
	}
	decoded, err := NewYAMLConfigDecoder().Decode(document)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got := decoded["version"]; got != 1 {
		t.Fatalf("decoded version = %#v, want 1", got)
	}
	if got := decoded["id"]; got != "example" {
		t.Fatalf("decoded id = %#v, want example", got)
	}
	config, err := stringMap(decoded["config"])
	if err != nil {
		t.Fatalf("decoded config error = %v", err)
	}
	if got := config["enabled"]; got != true {
		t.Fatalf("decoded config.enabled = %#v, want true", got)
	}
	if _, ok := decoded["config"].(map[string]any); !ok {
		t.Fatalf("decoded config type = %T, want map[string]any", decoded["config"])
	}
}

func TestYAMLConfigDecoderRejectsMalformedYAML(t *testing.T) {
	t.Parallel()

	_, err := NewYAMLConfigDecoder().Decode(SourceDefinitionDocument{
		Origin:  "/sources/broken.yaml",
		Content: []byte("items: [broken"),
	})
	if err == nil {
		t.Fatal("Decode() error = nil, want malformed YAML error")
	}
	if !strings.Contains(err.Error(), "/sources/broken.yaml") {
		t.Fatalf("Decode() error = %v, want origin context", err)
	}
}

func TestYAMLConfigDecoderRejectsEmptyDocument(t *testing.T) {
	t.Parallel()

	for _, content := range []string{"", "null\n"} {
		content := content
		t.Run(content, func(t *testing.T) {
			t.Parallel()
			_, err := NewYAMLConfigDecoder().Decode(SourceDefinitionDocument{Content: []byte(content)})
			if !errors.Is(err, ErrEmptySourceDefinition) {
				t.Fatalf("Decode() error = %v, want ErrEmptySourceDefinition", err)
			}
		})
	}
}

func TestYAMLConfigDecoderRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	_, err := NewYAMLConfigDecoder().Decode(SourceDefinitionDocument{
		Content: []byte("version: 1\n---\nversion: 1\n"),
	})
	if !errors.Is(err, ErrMultipleSourceDefinitions) {
		t.Fatalf("Decode() error = %v, want ErrMultipleSourceDefinitions", err)
	}
}
