// Package monitoring applies item-state semantics to successful collections.
package monitoring

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/suwa68/signal-watch/internal/itemkey"
	"github.com/suwa68/signal-watch/internal/source"
	"github.com/suwa68/signal-watch/internal/state"
)

var (
	ErrStateStoreRequired = errors.New("state store is required")
	ErrSourceIDRequired   = errors.New("source ID is required")
)

// UnseenItem carries the stable key needed to mark an item seen only after its
// future downstream processing succeeds.
type UnseenItem struct {
	Item source.MonitorItem
	Key  string
}

// Processor establishes a source baseline or returns items not yet seen.
type Processor struct {
	store state.StateStore
}

func NewProcessor(store state.StateStore) *Processor {
	return &Processor{store: store}
}

// ProcessSuccessfulCollection processes only a collection that the source
// adapter completed successfully. It never marks normal unseen items as seen.
func (p *Processor) ProcessSuccessfulCollection(
	ctx context.Context,
	sourceID string,
	items []source.MonitorItem,
) ([]UnseenItem, error) {
	if p == nil || p.store == nil {
		return nil, ErrStateStoreRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sourceID) == "" {
		return nil, ErrSourceIDRequired
	}

	candidates := make([]UnseenItem, len(items))
	keys := make([]string, len(items))
	for index, item := range items {
		key, err := itemkey.Build(item)
		if err != nil {
			return nil, fmt.Errorf("source %q: build key for item %d: %w", sourceID, index, err)
		}
		candidates[index] = UnseenItem{Item: item, Key: key}
		keys[index] = key
	}

	established, err := p.store.EstablishBaseline(ctx, sourceID, keys)
	if err != nil {
		return nil, fmt.Errorf("source %q: establish baseline: %w", sourceID, err)
	}
	if established {
		return []UnseenItem{}, nil
	}

	unseen := make([]UnseenItem, 0, len(candidates))
	queuedKeys := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		seen, err := p.store.HasItem(ctx, sourceID, candidate.Key)
		if err != nil {
			return nil, fmt.Errorf("source %q: check item %q: %w", sourceID, candidate.Key, err)
		}
		if seen {
			continue
		}
		if _, queued := queuedKeys[candidate.Key]; queued {
			continue
		}
		queuedKeys[candidate.Key] = struct{}{}
		unseen = append(unseen, candidate)
	}
	return unseen, nil
}
