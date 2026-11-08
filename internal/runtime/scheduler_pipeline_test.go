package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/suwa68/signal-watch/internal/application"
	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/source"
	"github.com/suwa68/signal-watch/internal/state"
)

type collectorFunc func(context.Context, config.SourceConfig) ([]source.MonitorItem, error)

func (f collectorFunc) Run(ctx context.Context, cfg config.SourceConfig) ([]source.MonitorItem, error) {
	return f(ctx, cfg)
}

type failingMarkStore struct {
	state.StateStore
	cause  error
	failed chan struct{}
}

func (s *failingMarkStore) MarkItemSeen(ctx context.Context, id, key string) error {
	if key == "external:C" {
		close(s.failed)
		return errors.Join(state.ErrFatal, s.cause)
	}
	return s.StateStore.MarkItemSeen(ctx, id, key)
}

// Pipeline intentionally continues to D after C's mark failure. Supervision
// cannot discover that failure before the complete RunOnce result is returned.
func TestRunDiscoversPipelineFatalOnlyAfterLaterItemCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &failingMarkStore{StateStore: state.NewMemoryStateStore(), cause: errors.New("mark C failed"), failed: make(chan struct{})}
	if _, err := store.EstablishBaseline(ctx, "news", nil); err != nil {
		t.Fatal(err)
	}
	dStarted := make(chan context.Context, 1)
	releaseD := make(chan struct{})
	pipeline, err := application.NewPipeline(collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
		return []source.MonitorItem{{ExternalID: "C", Title: "C"}, {ExternalID: "D", Title: "D"}}, nil
	}), store, application.NotificationSenderFunc(func(ctx context.Context, n notification.Notification) error {
		if n.Title == "D" {
			dStarted <- ctx
			select {
			case <-releaseD:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	siblingStarted, siblingStopped := make(chan struct{}), make(chan struct{})
	reports := make(chan error, 2)
	result := make(chan error, 1)
	go func() {
		result <- Run(ctx, []ScheduledSource{
			scheduled("news", 1, pipeline),
			scheduled("sibling", 1, runnerFunc(func(ctx context.Context, _ config.SourceConfig) error {
				close(siblingStarted)
				<-ctx.Done()
				close(siblingStopped)
				return ctx.Err()
			})),
		}, func(_ config.SourceConfig, err error) { reports <- err })
	}()
	receive(t, siblingStarted)
	receive(t, store.failed)
	dCtx := receive(t, dStarted)
	if dCtx.Err() != nil {
		t.Fatalf("supervision canceled before complete run: %v", dCtx.Err())
	}
	select {
	case err := <-reports:
		t.Fatalf("incomplete run reported: %v", err)
	default:
	}
	select {
	case <-siblingStopped:
		t.Fatal("sibling canceled before fatal result completed")
	default:
	}
	close(releaseD)
	err = receive(t, result)
	if !errors.Is(err, state.ErrFatal) || !errors.Is(err, store.cause) {
		t.Fatalf("fatal result lost: %v", err)
	}
	select {
	case <-siblingStopped:
	default:
		t.Fatal("sibling not joined")
	}
	seen, err := store.HasItem(context.Background(), "news", "external:D")
	if !seen || err != nil {
		t.Fatalf("later D was not marked: %v, %v", seen, err)
	}
}
