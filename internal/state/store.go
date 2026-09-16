// Package state defines item-state persistence boundaries.
package state

import "context"

// StateStore records source baselines and seen item keys.
type StateStore interface {
	IsSourceInitialized(ctx context.Context, sourceID string) (bool, error)

	// EstablishBaseline atomically records itemKeys and initializes sourceID.
	// It returns false without modifying state if the source was already
	// initialized.
	EstablishBaseline(ctx context.Context, sourceID string, itemKeys []string) (bool, error)

	HasItem(ctx context.Context, sourceID string, itemKey string) (bool, error)
	MarkItemSeen(ctx context.Context, sourceID string, itemKey string) error
}
