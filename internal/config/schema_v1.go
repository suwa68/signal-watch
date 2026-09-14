package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	ErrConfigVersionRequired    = errors.New("config version is required")
	ErrUnsupportedConfigVersion = errors.New("unsupported config version")
)

var sourceConfigV1Fields = map[string]struct{}{
	"version":          {},
	"id":               {},
	"name":             {},
	"type":             {},
	"interval_seconds": {},
	"config":           {},
}

// ParseSourceConfig validates schema version 1 and returns the generic runtime
// configuration. Schema version is intentionally not part of SourceConfig.
func ParseSourceConfig(decoded DecodedConfig) (SourceConfig, error) {
	if decoded == nil {
		return SourceConfig{}, errors.New("decoded source config is required")
	}

	if unknown := unknownTopLevelFields(decoded); len(unknown) > 0 {
		return SourceConfig{}, fmt.Errorf("unknown top-level field(s): %s", strings.Join(unknown, ", "))
	}

	versionValue, ok := decoded["version"]
	if !ok {
		return SourceConfig{}, ErrConfigVersionRequired
	}
	version, ok := versionValue.(int)
	if !ok {
		return SourceConfig{}, fmt.Errorf("config version must be an integer")
	}
	if version != 1 {
		return SourceConfig{}, fmt.Errorf("%w: %d", ErrUnsupportedConfigVersion, version)
	}

	id, err := requiredStringField(decoded, "id")
	if err != nil {
		return SourceConfig{}, err
	}
	name, err := requiredStringField(decoded, "name")
	if err != nil {
		return SourceConfig{}, err
	}
	sourceType, err := requiredStringField(decoded, "type")
	if err != nil {
		return SourceConfig{}, err
	}

	intervalValue, ok := decoded["interval_seconds"]
	if !ok {
		return SourceConfig{}, fmt.Errorf("field %q is required", "interval_seconds")
	}
	intervalSeconds, ok := intervalValue.(int)
	if !ok {
		return SourceConfig{}, fmt.Errorf("field %q must be an integer", "interval_seconds")
	}
	if intervalSeconds <= 0 {
		return SourceConfig{}, fmt.Errorf("field %q must be greater than zero", "interval_seconds")
	}

	configValue, ok := decoded["config"]
	if !ok {
		return SourceConfig{}, fmt.Errorf("field %q is required", "config")
	}
	adapterConfig, err := stringMap(configValue)
	if err != nil {
		return SourceConfig{}, fmt.Errorf("field %q must be an object: %w", "config", err)
	}

	return SourceConfig{
		ID:              id,
		Name:            name,
		Type:            sourceType,
		IntervalSeconds: intervalSeconds,
		Config:          adapterConfig,
	}, nil
}

func unknownTopLevelFields(decoded DecodedConfig) []string {
	unknown := make([]string, 0)
	for field := range decoded {
		if _, allowed := sourceConfigV1Fields[field]; !allowed {
			unknown = append(unknown, field)
		}
	}
	slices.Sort(unknown)
	return unknown
}

func requiredStringField(decoded DecodedConfig, field string) (string, error) {
	value, ok := decoded[field]
	if !ok {
		return "", fmt.Errorf("field %q is required", field)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("field %q must be a string", field)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("field %q must not be empty", field)
	}
	return text, nil
}

func stringMap(value any) (map[string]any, error) {
	switch typed := value.(type) {
	case DecodedConfig:
		result := make(map[string]any, len(typed))
		for key, entry := range typed {
			result[key] = entry
		}
		return result, nil
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, entry := range typed {
			result[key] = entry
		}
		return result, nil
	case map[any]any:
		result := make(map[string]any, len(typed))
		for key, entry := range typed {
			text, ok := key.(string)
			if !ok {
				return nil, errors.New("object keys must be strings")
			}
			result[text] = entry
		}
		return result, nil
	default:
		return nil, fmt.Errorf("got %T", value)
	}
}
