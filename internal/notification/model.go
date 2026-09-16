// Package notification defines the completed, transport-independent output of
// downstream notification processing.
package notification

import "time"

// Notification is the v1 delivery contract. Title, Summary, and SourceName are
// required plain text. URL and the source-provided PublishedAt are optional.
// Producing this model from MonitorItem remains a downstream concern.
type Notification struct {
	Title       string
	Summary     string
	SourceName  string
	URL         string
	PublishedAt *time.Time
}
