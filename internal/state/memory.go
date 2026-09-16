package state

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var (
	ErrSourceIDRequired = errors.New("source ID is required")
	ErrItemKeyRequired  = errors.New("item key is required")
)

// MemoryStateStore is a process-local, concurrency-safe StateStore. Its zero
// value is ready to use.
type MemoryStateStore struct {
	mu                 sync.RWMutex
	initializedSources map[string]struct{}
	seenItems          map[string]map[string]struct{}
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		initializedSources: make(map[string]struct{}),
		seenItems:          make(map[string]map[string]struct{}),
	}
}

func (s *MemoryStateStore) IsSourceInitialized(ctx context.Context, sourceID string) (bool, error) {
	if err := validateContextAndSource(ctx, sourceID); err != nil {
		return false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, initialized := s.initializedSources[sourceID]
	return initialized, nil
}

func (s *MemoryStateStore) EstablishBaseline(ctx context.Context, sourceID string, itemKeys []string) (bool, error) {
	if err := validateContextAndSource(ctx, sourceID); err != nil {
		return false, err
	}
	for _, itemKey := range itemKeys {
		if !usable(itemKey) {
			return false, ErrItemKeyRequired
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, initialized := s.initializedSources[sourceID]; initialized {
		return false, nil
	}

	s.ensureMaps()
	items := s.seenItems[sourceID]
	if items == nil {
		items = make(map[string]struct{}, len(itemKeys))
		s.seenItems[sourceID] = items
	}
	for _, itemKey := range itemKeys {
		items[itemKey] = struct{}{}
	}
	s.initializedSources[sourceID] = struct{}{}
	return true, nil
}

func (s *MemoryStateStore) HasItem(ctx context.Context, sourceID, itemKey string) (bool, error) {
	if err := validateContextSourceAndKey(ctx, sourceID, itemKey); err != nil {
		return false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, seen := s.seenItems[sourceID][itemKey]
	return seen, nil
}

func (s *MemoryStateStore) MarkItemSeen(ctx context.Context, sourceID, itemKey string) error {
	if err := validateContextSourceAndKey(ctx, sourceID, itemKey); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.ensureMaps()
	items := s.seenItems[sourceID]
	if items == nil {
		items = make(map[string]struct{})
		s.seenItems[sourceID] = items
	}
	items[itemKey] = struct{}{}
	return nil
}

func (s *MemoryStateStore) ensureMaps() {
	if s.initializedSources == nil {
		s.initializedSources = make(map[string]struct{})
	}
	if s.seenItems == nil {
		s.seenItems = make(map[string]map[string]struct{})
	}
}

func validateContextAndSource(ctx context.Context, sourceID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !usable(sourceID) {
		return ErrSourceIDRequired
	}
	return nil
}

func validateContextSourceAndKey(ctx context.Context, sourceID, itemKey string) error {
	if err := validateContextAndSource(ctx, sourceID); err != nil {
		return err
	}
	if !usable(itemKey) {
		return ErrItemKeyRequired
	}
	return nil
}

func usable(value string) bool {
	return strings.TrimSpace(value) != ""
}

var _ StateStore = (*MemoryStateStore)(nil)
