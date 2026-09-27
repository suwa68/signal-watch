package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/source"
)

const (
	Version                = 1
	DefaultMaxItems        = 20
	MinMaxItems            = 1
	MaxMaxItems            = 100
	DefaultMaxContentChars = 1000
	MinMaxContentChars     = 100
	MaxMaxContentChars     = 5000
)

// Collector is the source-side contract used by queries.
type Collector interface {
	Run(context.Context, config.SourceConfig) ([]source.MonitorItem, error)
}

// Limits bounds only the returned result, not fetch or parse cost.
type Limits struct {
	MaxItems        int
	MaxContentChars int
}

func DefaultLimits() Limits {
	return Limits{MaxItems: DefaultMaxItems, MaxContentChars: DefaultMaxContentChars}
}

func (l Limits) Validate() error {
	if l.MaxItems < MinMaxItems || l.MaxItems > MaxMaxItems {
		return fmt.Errorf("max_items must be between %d and %d", MinMaxItems, MaxMaxItems)
	}
	if l.MaxContentChars < MinMaxContentChars || l.MaxContentChars > MaxMaxContentChars {
		return fmt.Errorf("max_content_chars must be between %d and %d", MinMaxContentChars, MaxMaxContentChars)
	}
	return nil
}

type SourceDescriptor struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Type                string  `json:"type"`
	DefinitionRevision  string  `json:"definition_revision"`
	ConfiguredSourceURL *string `json:"configured_source_url"`
}

type Item struct {
	SourceID            string     `json:"source_id"`
	ExternalID          string     `json:"external_id"`
	Title               string     `json:"title"`
	URL                 string     `json:"url"`
	Content             string     `json:"content"`
	ContentTruncated    bool       `json:"content_truncated"`
	ContentStatus       string     `json:"content_status"`
	ContentCompleteness string     `json:"content_completeness"`
	PublishedAt         *time.Time `json:"published_at"`
}

type Snapshot struct {
	Version               int              `json:"version"`
	Source                SourceDescriptor `json:"source"`
	CollectionStartedAt   time.Time        `json:"collection_started_at"`
	CollectionCompletedAt time.Time        `json:"collection_completed_at"`
	TotalItems            int              `json:"total_items"`
	ReturnedItems         int              `json:"returned_items"`
	Truncated             bool             `json:"truncated"`
	Items                 []Item           `json:"items"`
}

type CatalogSource struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Type               string `json:"type"`
	DefinitionRevision string `json:"definition_revision"`
}

type CatalogResponse struct {
	Version int             `json:"version"`
	Sources []CatalogSource `json:"sources"`
}

// Service collects one configured source without seen state or notifications.
type Service struct {
	collector Collector
	now       func() time.Time
}

func NewService(collector Collector) (*Service, error) {
	if collector == nil {
		return nil, errors.New("collector is required")
	}
	return &Service{collector: collector, now: time.Now}, nil
}

// CatalogResult maps the immutable catalog to its public discovery contract.
func CatalogResult(catalog *Catalog) CatalogResponse {
	definitions := catalog.Definitions()
	sources := make([]CatalogSource, 0, len(definitions))
	for _, definition := range definitions {
		sources = append(sources, CatalogSource{
			ID: definition.Config.ID, Name: definition.Config.Name, Type: definition.Config.Type,
			DefinitionRevision: definition.DefinitionRevision,
		})
	}
	return CatalogResponse{Version: Version, Sources: sources}
}

// Collect invokes the adapter once and maps the complete result into Query JSON v1.
func (s *Service) Collect(ctx context.Context, definition Definition, limits Limits) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if s == nil || s.collector == nil {
		return Snapshot{}, errors.New("query service is not initialized")
	}
	if err := limits.Validate(); err != nil {
		return Snapshot{}, err
	}
	started := s.now().UTC()
	items, err := s.collector.Run(ctx, definition.Config)
	if err != nil {
		return Snapshot{}, fmt.Errorf("collect source %q: %w", definition.Config.ID, err)
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	completed := s.now().UTC()

	returned := len(items)
	if returned > limits.MaxItems {
		returned = limits.MaxItems
	}
	output := make([]Item, 0, returned)
	for _, candidate := range items[:returned] {
		content, truncated := truncateRunes(candidate.Content, limits.MaxContentChars)
		status := "present"
		if strings.TrimSpace(content) == "" {
			status = "empty"
		}
		var publishedAt *time.Time
		if candidate.PublishedAt != nil {
			utc := candidate.PublishedAt.UTC()
			publishedAt = &utc
		}
		output = append(output, Item{
			SourceID: candidate.SourceID, ExternalID: candidate.ExternalID,
			Title: candidate.Title, URL: candidate.URL, Content: content,
			ContentTruncated: truncated, ContentStatus: status,
			ContentCompleteness: "unknown", PublishedAt: publishedAt,
		})
	}
	return Snapshot{
		Version: Version,
		Source: SourceDescriptor{
			ID: definition.Config.ID, Name: definition.Config.Name, Type: definition.Config.Type,
			DefinitionRevision:  definition.DefinitionRevision,
			ConfiguredSourceURL: definition.ConfiguredSourceURL,
		},
		CollectionStartedAt: started, CollectionCompletedAt: completed,
		TotalItems: len(items), ReturnedItems: returned,
		Truncated: len(items) > returned, Items: output,
	}, nil
}

func truncateRunes(value string, limit int) (string, bool) {
	if utf8.RuneCountInString(value) <= limit {
		return value, false
	}
	runes := []rune(value)
	return string(runes[:limit]), true
}
