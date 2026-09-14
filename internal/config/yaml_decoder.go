package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

var (
	ErrEmptySourceDefinition     = errors.New("source definition is empty")
	ErrMultipleSourceDefinitions = errors.New("multiple YAML documents are not supported")
)

// YAMLConfigDecoder decodes exactly one YAML document into generic values.
type YAMLConfigDecoder struct{}

func NewYAMLConfigDecoder() *YAMLConfigDecoder {
	return &YAMLConfigDecoder{}
}

func (d *YAMLConfigDecoder) Decode(document SourceDefinitionDocument) (DecodedConfig, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(document.Content))
	var decoded DecodedConfig
	if err := decoder.Decode(&decoded); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("decode %s: %w", documentLabel(document), ErrEmptySourceDefinition)
		}
		return nil, fmt.Errorf("decode %s as YAML: %w", documentLabel(document), err)
	}
	if decoded == nil {
		return nil, fmt.Errorf("decode %s: %w", documentLabel(document), ErrEmptySourceDefinition)
	}

	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("decode %s: %w", documentLabel(document), ErrMultipleSourceDefinitions)
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode trailing YAML in %s: %w", documentLabel(document), err)
	}

	normalized, err := normalizeDecodedConfig(decoded)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", documentLabel(document), err)
	}
	return normalized, nil
}

func normalizeDecodedConfig(decoded DecodedConfig) (DecodedConfig, error) {
	normalized := make(DecodedConfig, len(decoded))
	for key, value := range decoded {
		entry, err := normalizeDecodedValue(value)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
		normalized[key] = entry
	}
	return normalized, nil
}

func normalizeDecodedValue(value any) (any, error) {
	switch typed := value.(type) {
	case DecodedConfig:
		result := make(map[string]any, len(typed))
		for key, entry := range typed {
			normalized, err := normalizeDecodedValue(entry)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", key, err)
			}
			result[key] = normalized
		}
		return result, nil
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, entry := range typed {
			normalized, err := normalizeDecodedValue(entry)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", key, err)
			}
			result[key] = normalized
		}
		return result, nil
	case map[any]any:
		result := make(map[string]any, len(typed))
		for key, entry := range typed {
			text, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("object key %v must be a string", key)
			}
			normalized, err := normalizeDecodedValue(entry)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", text, err)
			}
			result[text] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, entry := range typed {
			normalized, err := normalizeDecodedValue(entry)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", index, err)
			}
			result[index] = normalized
		}
		return result, nil
	default:
		return value, nil
	}
}

func documentLabel(document SourceDefinitionDocument) string {
	if document.Origin != "" {
		return fmt.Sprintf("source definition %q", document.Origin)
	}
	if document.Key != "" {
		return fmt.Sprintf("source definition %q", document.Key)
	}
	return "source definition"
}

var _ ConfigDecoder = (*YAMLConfigDecoder)(nil)
