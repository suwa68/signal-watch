package source

import (
	"context"
	"errors"

	"github.com/suwa68/signal-watch/internal/config"
)

var errSourceAdapterRegistryRequired = errors.New("source adapter registry is required")

// SourceRunner resolves and invokes the adapter for a source configuration.
type SourceRunner struct {
	registry *SourceAdapterRegistry
}

// NewSourceRunner creates a runner backed by registry.
func NewSourceRunner(registry *SourceAdapterRegistry) *SourceRunner {
	return &SourceRunner{registry: registry}
}

// Run collects items with the adapter registered for source.Type.
func (r *SourceRunner) Run(ctx context.Context, source config.SourceConfig) ([]MonitorItem, error) {
	if r == nil || r.registry == nil {
		return nil, errSourceAdapterRegistryRequired
	}

	adapter, err := r.registry.Lookup(source.Type)
	if err != nil {
		return nil, err
	}

	return adapter.Collect(ctx, source)
}
