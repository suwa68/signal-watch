package source

import "time"

// MonitorItem is the provisional normalized output of a source adapter.
//
// ExternalID and PublishedAt are optional source-provided identity candidates.
// Item keys and seen-state remain downstream concerns, and this type is not a
// final cross-language schema.
type MonitorItem struct {
	SourceID   string
	ExternalID string

	Title   string
	URL     string
	Content string

	PublishedAt *time.Time
}
