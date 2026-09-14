package source

import (
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrUnsupportedSourceType indicates that no adapter is registered for a
	// source type.
	ErrUnsupportedSourceType = errors.New("unsupported source type")

	// ErrNilSourceAdapter indicates an attempt to register a nil adapter.
	ErrNilSourceAdapter = errors.New("source adapter is nil")

	// ErrSourceAdapterAlreadyRegistered indicates an attempt to replace an
	// existing registration. Registrations are explicit and immutable.
	ErrSourceAdapterAlreadyRegistered = errors.New("source adapter already registered")
)

// SourceAdapterRegistry resolves a source type to its adapter.
//
// Each registry owns its registrations. The zero value is ready to use.
type SourceAdapterRegistry struct {
	mu       sync.RWMutex
	adapters map[string]SourceAdapter
}

// NewSourceAdapterRegistry creates an empty registry.
func NewSourceAdapterRegistry() *SourceAdapterRegistry {
	return &SourceAdapterRegistry{
		adapters: make(map[string]SourceAdapter),
	}
}

// Register associates sourceType with adapter. Existing registrations cannot
// be replaced implicitly.
func (r *SourceAdapterRegistry) Register(sourceType string, adapter SourceAdapter) error {
	if adapter == nil {
		return ErrNilSourceAdapter
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.adapters == nil {
		r.adapters = make(map[string]SourceAdapter)
	}
	if _, exists := r.adapters[sourceType]; exists {
		return fmt.Errorf("%w: %q", ErrSourceAdapterAlreadyRegistered, sourceType)
	}

	r.adapters[sourceType] = adapter
	return nil
}

// Lookup returns the adapter registered for sourceType.
func (r *SourceAdapterRegistry) Lookup(sourceType string) (SourceAdapter, error) {
	r.mu.RLock()
	adapter, exists := r.adapters[sourceType]
	r.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedSourceType, sourceType)
	}

	return adapter, nil
}
