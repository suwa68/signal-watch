// Package notification defines the completed, transport-independent output of
// downstream notification processing.
package notification

import "time"

// Notification is the v1 delivery contract. Title and SourceName are required
// plain text. Summary is optional; empty or whitespace-only means absent.
// All supplied text must be valid UTF-8. URL and the source-provided PublishedAt
// are optional.
// Producing this model from MonitorItem remains a downstream concern.
type Notification struct {
	Title       string
	Summary     string
	SourceName  string
	URL         string
	PublishedAt *time.Time
}
