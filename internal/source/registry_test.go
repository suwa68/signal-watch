package source

import (
	"context"
	"errors"
	"testing"

	"github.com/suwa68/signal-watch/internal/config"
)

func TestSourceAdapterRegistryRegisterAndLookup(t *testing.T) {
	t.Parallel()

	registry := NewSourceAdapterRegistry()
	adapter := &fakeAdapter{}

	if err := registry.Register("html", adapter); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := registry.Lookup("html")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got != adapter {
		t.Fatalf("Lookup() = %T %p, want %T %p", got, got, adapter, adapter)
	}
}

func TestSourceAdapterRegistryZeroValue(t *testing.T) {
	t.Parallel()

	var registry SourceAdapterRegistry
	adapter := &fakeAdapter{}

	if err := registry.Register("html", adapter); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if _, err := registry.Lookup("html"); err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
}

func TestSourceAdapterRegistryRejectsNilAdapter(t *testing.T) {
	t.Parallel()

	registry := NewSourceAdapterRegistry()
	err := registry.Register("html", nil)

	if !errors.Is(err, ErrNilSourceAdapter) {
		t.Fatalf("Register() error = %v, want ErrNilSourceAdapter", err)
	}
	if _, err := registry.Lookup("html"); !errors.Is(err, ErrUnsupportedSourceType) {
		t.Fatalf("Lookup() after rejected registration error = %v, want ErrUnsupportedSourceType", err)
	}
}

func TestSourceAdapterRegistryRejectsDuplicateRegistration(t *testing.T) {
	t.Parallel()

	registry := NewSourceAdapterRegistry()
	first := &fakeAdapter{}
	second := &fakeAdapter{}

	if err := registry.Register("html", first); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	err := registry.Register("html", second)
	if !errors.Is(err, ErrSourceAdapterAlreadyRegistered) {
		t.Fatalf("second Register() error = %v, want ErrSourceAdapterAlreadyRegistered", err)
	}

	got, err := registry.Lookup("html")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got != first {
		t.Fatal("duplicate registration replaced the original adapter")
	}
}

func TestSourceAdapterRegistryUnsupportedType(t *testing.T) {
	t.Parallel()

	registry := NewSourceAdapterRegistry()
	_, err := registry.Lookup("rss")

	if !errors.Is(err, ErrUnsupportedSourceType) {
		t.Fatalf("Lookup() error = %v, want ErrUnsupportedSourceType", err)
	}
}

type fakeAdapter struct {
	collect func(context.Context, config.SourceConfig) ([]MonitorItem, error)
}

func (a *fakeAdapter) Collect(ctx context.Context, source config.SourceConfig) ([]MonitorItem, error) {
	if a.collect == nil {
		return nil, nil
	}
	return a.collect(ctx, source)
}
