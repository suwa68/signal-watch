package source

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/suwa68/signal-watch/internal/config"
)

func TestSourceRunnerDispatchesAndPropagatesResultAndContext(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "request-1")
	sourceConfig := config.SourceConfig{
		ID:   "example",
		Name: "Example",
		Type: "html",
	}
	wantItems := []MonitorItem{{
		SourceID: "example",
		Title:    "First item",
		URL:      "https://example.com/first",
		Content:  "Summary",
	}}

	registry := NewSourceAdapterRegistry()
	adapter := &fakeAdapter{collect: func(gotContext context.Context, gotSource config.SourceConfig) ([]MonitorItem, error) {
		if gotContext != ctx {
			t.Error("Collect() did not receive the runner context")
		}
		if !reflect.DeepEqual(gotSource, sourceConfig) {
			t.Errorf("Collect() source = %#v, want %#v", gotSource, sourceConfig)
		}
		return wantItems, nil
	}}
	if err := registry.Register("html", adapter); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	gotItems, err := NewSourceRunner(registry).Run(ctx, sourceConfig)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !reflect.DeepEqual(gotItems, wantItems) {
		t.Fatalf("Run() = %#v, want %#v", gotItems, wantItems)
	}
}

func TestSourceRunnerPropagatesAdapterError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("collection failed")
	registry := NewSourceAdapterRegistry()
	if err := registry.Register("html", &fakeAdapter{collect: func(context.Context, config.SourceConfig) ([]MonitorItem, error) {
		return nil, wantErr
	}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	_, err := NewSourceRunner(registry).Run(context.Background(), config.SourceConfig{Type: "html"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want adapter error", err)
	}
}

func TestSourceRunnerPropagatesCancellation(t *testing.T) {
	t.Parallel()

	registry := NewSourceAdapterRegistry()
	if err := registry.Register("html", &fakeAdapter{collect: func(ctx context.Context, _ config.SourceConfig) ([]MonitorItem, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewSourceRunner(registry).Run(ctx, config.SourceConfig{Type: "html"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestSourceRunnerReturnsUnsupportedTypeError(t *testing.T) {
	t.Parallel()

	_, err := NewSourceRunner(NewSourceAdapterRegistry()).Run(
		context.Background(),
		config.SourceConfig{Type: "rss"},
	)

	if !errors.Is(err, ErrUnsupportedSourceType) {
		t.Fatalf("Run() error = %v, want ErrUnsupportedSourceType", err)
	}
}
