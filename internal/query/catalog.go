// Package query exposes current source data without monitoring state or delivery.
package query

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/source"
)

var (
	ErrSourceNotFound = errors.New("source not found")
	ErrDuplicateID    = errors.New("duplicate source ID")
)

// Definition contains a validated configuration and metadata from the exact
// definition bytes that produced it.
type Definition struct {
	Config              config.SourceConfig
	DefinitionRevision  string
	ConfiguredSourceURL *string
}

// DefinitionMetadata contains adapter-specific public metadata for Query v1.
type DefinitionMetadata struct {
	ConfiguredSourceURL *string
}

// DefinitionValidator validates an adapter-specific configuration before any
// collection starts and returns safe metadata for query responses.
type DefinitionValidator interface {
	Validate(config.SourceConfig) (DefinitionMetadata, error)
}

// DefinitionValidatorFunc adapts a function into a DefinitionValidator.
type DefinitionValidatorFunc func(config.SourceConfig) (DefinitionMetadata, error)

func (f DefinitionValidatorFunc) Validate(cfg config.SourceConfig) (DefinitionMetadata, error) {
	return f(cfg)
}

// Catalog is an immutable set of definitions loaded and validated together.
type Catalog struct {
	byID    map[string]Definition
	ordered []Definition
}

// LoadCatalog loads every definition and rejects the entire catalog on any
// invalid document, duplicate ID, unregistered type, or adapter settings error.
func LoadCatalog(
	ctx context.Context,
	repository config.SourceDefinitionRepository,
	decoder config.ConfigDecoder,
	registry *source.SourceAdapterRegistry,
	validators map[string]DefinitionValidator,
) (*Catalog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if repository == nil || decoder == nil || registry == nil {
		return nil, errors.New("repository, decoder, and adapter registry are required")
	}
	documents, err := repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list source definitions: %w", err)
	}

	definitions := make([]Definition, 0, len(documents))
	byID := make(map[string]Definition, len(documents))
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		decoded, err := decoder.Decode(document)
		if err != nil {
			return nil, err
		}
		cfg, err := config.ParseSourceConfig(decoded)
		if err != nil {
			return nil, fmt.Errorf("validate source definition %q: %w", document.Key, err)
		}
		if _, exists := byID[cfg.ID]; exists {
			return nil, fmt.Errorf("%w %q", ErrDuplicateID, cfg.ID)
		}
		if _, err := registry.Lookup(cfg.Type); err != nil {
			return nil, fmt.Errorf("validate source %q: %w", cfg.ID, err)
		}
		validator, ok := validators[cfg.Type]
		if !ok || validator == nil {
			return nil, fmt.Errorf("validate source %q: no definition validator for registered type %q", cfg.ID, cfg.Type)
		}
		metadata, err := validator.Validate(cfg)
		if err != nil {
			return nil, fmt.Errorf("validate source %q: %w", cfg.ID, err)
		}
		definition := Definition{
			Config:              cfg,
			DefinitionRevision:  document.Revision,
			ConfiguredSourceURL: metadata.ConfiguredSourceURL,
		}
		byID[cfg.ID] = definition
		definitions = append(definitions, definition)
	}
	slices.SortFunc(definitions, func(left, right Definition) int {
		return strings.Compare(left.Config.ID, right.Config.ID)
	})
	return &Catalog{byID: byID, ordered: definitions}, nil
}

// Lookup returns one loaded definition by exact source ID.
func (c *Catalog) Lookup(sourceID string) (Definition, error) {
	if c == nil {
		return Definition{}, errors.New("source catalog is required")
	}
	definition, ok := c.byID[sourceID]
	if !ok {
		return Definition{}, fmt.Errorf("%w: %q", ErrSourceNotFound, sourceID)
	}
	return definition, nil
}

// Definitions returns the catalog's definitions in source ID order.
func (c *Catalog) Definitions() []Definition {
	if c == nil || len(c.ordered) == 0 {
		return []Definition{}
	}
	return slices.Clone(c.ordered)
}
