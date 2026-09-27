package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/suwa68/signal-watch/internal/application"
	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/itemkey"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/source"
	"github.com/suwa68/signal-watch/internal/state"
)

type collectorFunc func(context.Context, config.SourceConfig) ([]source.MonitorItem, error)

func (f collectorFunc) Run(ctx context.Context, cfg config.SourceConfig) ([]source.MonitorItem, error) {
	return f(ctx, cfg)
}

func sourceConfig() config.SourceConfig {
	return config.SourceConfig{ID: "news", Name: "News <&>", Type: "test"}
}

func items(names ...string) []source.MonitorItem {
	result := make([]source.MonitorItem, len(names))
	for i, name := range names {
		// State must use configured source ID, not trust an adapter's SourceID.
		result[i] = source.MonitorItem{SourceID: "adapter-id", ExternalID: name, Title: name}
	}
	return result
}

func newPipeline(t *testing.T, collector application.Collector, store state.StateStore, sender application.NotificationSender) *application.Pipeline {
	t.Helper()
	pipeline, err := application.NewPipeline(collector, store, sender)
	if err != nil {
		t.Fatal(err)
	}
	return pipeline
}

func runOK(t *testing.T, pipeline *application.Pipeline) {
	t.Helper()
	if err := pipeline.RunOnce(context.Background(), sourceConfig()); err != nil {
		t.Fatal(err)
	}
}

func assertSeen(t *testing.T, store state.StateStore, name string, want bool) {
	t.Helper()
	got, err := store.HasItem(context.Background(), sourceConfig().ID, "external:"+name)
	if err != nil || got != want {
		t.Fatalf("seen(%s) = %v, %v; want %v", name, got, err, want)
	}
}

type controlledStore struct {
	*state.MemoryStateStore
	establishCalls int
	has            func(context.Context, string, string) (bool, error)
	mark           func(context.Context, string, string) error
}

func (s *controlledStore) EstablishBaseline(ctx context.Context, id string, keys []string) (bool, error) {
	s.establishCalls++
	return s.MemoryStateStore.EstablishBaseline(ctx, id, keys)
}

func (s *controlledStore) HasItem(ctx context.Context, id, key string) (bool, error) {
	if s.has != nil {
		return s.has(ctx, id, key)
	}
	return s.MemoryStateStore.HasItem(ctx, id, key)
}

func (s *controlledStore) MarkItemSeen(ctx context.Context, id, key string) error {
	if s.mark != nil {
		return s.mark(ctx, id, key)
	}
	return s.MemoryStateStore.MarkItemSeen(ctx, id, key)
}

func TestNewPipelineRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	collector := collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) { return nil, nil })
	sender := application.NotificationSenderFunc(func(context.Context, notification.Notification) error { return nil })
	store := state.NewMemoryStateStore()
	for _, tt := range []struct {
		name      string
		collector application.Collector
		store     state.StateStore
		sender    application.NotificationSender
	}{
		{"collector", nil, store, sender},
		{"typed nil collector", collectorFunc(nil), store, sender},
		{"store", collector, nil, sender},
		{"typed nil store", collector, (*state.MemoryStateStore)(nil), sender},
		{"sender", collector, store, nil},
		{"typed nil sender", collector, store, application.NotificationSenderFunc(nil)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := application.NewPipeline(tt.collector, tt.store, tt.sender); err == nil || got != nil {
				t.Fatalf("NewPipeline() = %v, %v", got, err)
			}
		})
	}
}

func TestRunOnceRejectsInvalidSourceAndCanceledCallerBeforeCollection(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		cfg  config.SourceConfig
		ctx  func() (context.Context, context.CancelFunc)
		want error
	}{
		{name: "blank ID", cfg: config.SourceConfig{ID: " \t", Name: "News"}},
		{name: "blank name", cfg: config.SourceConfig{ID: "news", Name: " \n"}},
		{name: "invalid name UTF8", cfg: config.SourceConfig{ID: "news", Name: "\xff"}},
		{name: "canceled", cfg: sourceConfig(), ctx: func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, want: context.Canceled},
		{name: "deadline", cfg: sourceConfig(), ctx: func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Unix(1, 0))
		}, want: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &controlledStore{MemoryStateStore: state.NewMemoryStateStore()}
			pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
				t.Error("invalid call reached collector")
				return nil, nil
			}), store, application.NotificationSenderFunc(func(context.Context, notification.Notification) error {
				t.Error("invalid call reached sender")
				return nil
			}))
			ctx := context.Background()
			if tt.ctx != nil {
				var cancel context.CancelFunc
				ctx, cancel = tt.ctx()
				defer cancel()
			}
			err := pipeline.RunOnce(ctx, tt.cfg)
			if err == nil || (tt.want != nil && !errors.Is(err, tt.want)) || store.establishCalls != 0 {
				t.Fatalf("error=%v, processor calls=%d", err, store.establishCalls)
			}
		})
	}
}

func TestRunOnceBaselineDeliveryAndSuppression(t *testing.T) {
	t.Parallel()
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty=%v", empty), func(t *testing.T) {
			store := state.NewMemoryStateStore()
			batch := items("A", "B")
			if empty {
				batch = nil
			}
			var sent []string
			pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
				return batch, nil
			}), store, application.NotificationSenderFunc(func(_ context.Context, n notification.Notification) error {
				assertSeen(t, store, n.Title, false)
				sent = append(sent, n.Title)
				return nil
			}))
			runOK(t, pipeline)
			initialized, err := store.IsSourceInitialized(context.Background(), sourceConfig().ID)
			if err != nil || !initialized || len(sent) != 0 {
				t.Fatalf("baseline initialized=%v, error=%v, sent=%v", initialized, err, sent)
			}
			batch = append(batch, items("C", "C")...)
			runOK(t, pipeline)
			assertSeen(t, store, "C", true)
			runOK(t, pipeline)
			if !reflect.DeepEqual(sent, []string{"C"}) {
				t.Fatalf("sent=%v, want C exactly once", sent)
			}
			if got, err := store.HasItem(context.Background(), "adapter-id", "external:C"); err != nil || got {
				t.Fatalf("state leaked to adapter ID: %v, %v", got, err)
			}
		})
	}
}

func TestCollectionAndIdentityFailuresDoNotInitialize(t *testing.T) {
	t.Parallel()
	collectErr := errors.New("collection failed")
	for _, tt := range []struct {
		name       string
		batch      []source.MonitorItem
		collectErr error
		want       error
	}{
		{"collection", nil, collectErr, collectErr},
		{"partial collection", items("A"), collectErr, collectErr},
		{"invalid identity", append(items("A"), source.MonitorItem{}), nil, itemkey.ErrNoDeterministicIdentity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &controlledStore{MemoryStateStore: state.NewMemoryStateStore()}
			batch, collectErr := tt.batch, tt.collectErr
			pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
				return batch, collectErr
			}), store, application.NotificationSenderFunc(func(context.Context, notification.Notification) error {
				t.Error("baseline failure or recovery must not send")
				return nil
			}))
			if err := pipeline.RunOnce(context.Background(), sourceConfig()); !errors.Is(err, tt.want) {
				t.Fatalf("error=%v, want %v", err, tt.want)
			}
			initialized, err := store.IsSourceInitialized(context.Background(), sourceConfig().ID)
			if initialized || err != nil || store.establishCalls != 0 {
				t.Fatalf("failure mutated baseline: initialized=%v, error=%v, calls=%d", initialized, err, store.establishCalls)
			}
			assertSeen(t, store, "A", false)
			batch, collectErr = items("A", "B"), nil
			runOK(t, pipeline) // Guard is released; next success is a silent baseline.
			assertSeen(t, store, "A", true)
			assertSeen(t, store, "B", true)
		})
	}
}

func TestProcessingFailureDoesNotDeliverEarlierCandidates(t *testing.T) {
	t.Parallel()
	store := &controlledStore{MemoryStateStore: state.NewMemoryStateStore()}
	batch := items("A")
	pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
		return batch, nil
	}), store, application.NotificationSenderFunc(func(context.Context, notification.Notification) error {
		t.Error("failed processing attempt must not deliver")
		return nil
	}))
	runOK(t, pipeline)
	wantErr := errors.New("state read failed")
	store.has = func(ctx context.Context, id, key string) (bool, error) {
		if key == "external:D" {
			return false, wantErr
		}
		return store.MemoryStateStore.HasItem(ctx, id, key)
	}
	batch = items("C", "D")
	if err := pipeline.RunOnce(context.Background(), sourceConfig()); !errors.Is(err, wantErr) {
		t.Fatalf("error=%v, want state read failure", err)
	}
	assertSeen(t, store.MemoryStateStore, "C", false)
	assertSeen(t, store.MemoryStateStore, "D", false)
}

func TestMappingPreservesMetadataWithoutNLP(t *testing.T) {
	t.Parallel()
	for _, optional := range []bool{false, true} {
		store := state.NewMemoryStateStore()
		// Invalid display text with a valid key remains valid historical baseline.
		batch := []source.MonitorItem{{ExternalID: "historical", Title: "\xff"}}
		var got []notification.Notification
		pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
			return batch, nil
		}), store, application.NotificationSenderFunc(func(_ context.Context, n notification.Notification) error {
			got = append(got, n)
			return nil
		}))
		runOK(t, pipeline)
		item := items("C")[0]
		item.Title, item.Content = "Original <title> & 中文", "Never use this as an NLP summary"
		if optional {
			published := time.Date(2026, 9, 27, 10, 30, 0, 0, time.FixedZone("CST", 8*60*60))
			item.URL, item.PublishedAt = "https://example.com/?a=1&b=2", &published
		}
		batch = []source.MonitorItem{item}
		runOK(t, pipeline)
		want := notification.Notification{Title: item.Title, SourceName: sourceConfig().Name, URL: item.URL, PublishedAt: item.PublishedAt}
		if !reflect.DeepEqual(got, []notification.Notification{want}) {
			t.Fatalf("mapped=%#v, want %#v", got, want)
		}
		assertSeen(t, store, "C", true)
	}
}

func TestInvalidUnseenTitleLeavesItemUnseenAndContinues(t *testing.T) {
	t.Parallel()
	for _, title := range []string{"", " \t", "\xff"} {
		store := state.NewMemoryStateStore()
		var batch []source.MonitorItem
		var sent []string
		pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
			return batch, nil
		}), store, application.NotificationSenderFunc(func(_ context.Context, n notification.Notification) error {
			sent = append(sent, n.Title)
			return nil
		}))
		runOK(t, pipeline)
		batch = items("C", "D")
		batch[0].Title = title
		err := pipeline.RunOnce(context.Background(), sourceConfig())
		if err == nil || !strings.Contains(err.Error(), `item "external:C": map notification`) || !reflect.DeepEqual(sent, []string{"D"}) {
			t.Fatalf("error=%v, sent=%v", err, sent)
		}
		assertSeen(t, store, "C", false)
		assertSeen(t, store, "D", true)
	}
}

type deliveryFailure struct{ cause error }

func (e *deliveryFailure) Error() string { return "sender failure: " + e.cause.Error() }
func (e *deliveryFailure) Unwrap() error { return e.cause }

func TestSenderLocalFailureContinuesWhileCallerActive(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{errors.New("transport failed"), context.DeadlineExceeded, context.Canceled} {
		t.Run(cause.Error(), func(t *testing.T) {
			store := state.NewMemoryStateStore()
			batch := items("A", "B")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := &deliveryFailure{cause: cause}
			sendErr := fmt.Errorf("send C: %w", failure)
			failC := true
			var sent []string
			pipeline := newPipeline(t, collectorFunc(func(gotCtx context.Context, cfg config.SourceConfig) ([]source.MonitorItem, error) {
				if gotCtx != ctx || !reflect.DeepEqual(cfg, sourceConfig()) {
					t.Error("collector did not receive caller context/config")
				}
				return batch, nil
			}), store, application.NotificationSenderFunc(func(gotCtx context.Context, n notification.Notification) error {
				if gotCtx != ctx {
					t.Error("sender did not receive caller context")
				}
				assertSeen(t, store, n.Title, false)
				sent = append(sent, n.Title)
				if n.Title == "C" && failC {
					return sendErr
				}
				return nil
			}))
			if err := pipeline.RunOnce(ctx, sourceConfig()); err != nil || len(sent) != 0 {
				t.Fatalf("baseline: error=%v, sent=%v", err, sent)
			}
			batch = items("A", "B", "C", "D")
			err := pipeline.RunOnce(ctx, sourceConfig())
			var gotFailure *deliveryFailure
			if !errors.Is(err, sendErr) || !errors.Is(err, cause) || !errors.As(err, &gotFailure) || gotFailure != failure {
				t.Fatalf("aggregate lost sender failure: %v", err)
			}
			if ctx.Err() != nil || !reflect.DeepEqual(sent, []string{"C", "D"}) {
				t.Fatalf("caller error=%v, sent=%v; want C and D", ctx.Err(), sent)
			}
			if !strings.Contains(err.Error(), `source "news" item "external:C": send notification`) {
				t.Fatalf("missing error context: %v", err)
			}
			assertSeen(t, store, "C", false)
			assertSeen(t, store, "D", true)
			failC = false
			if err := pipeline.RunOnce(ctx, sourceConfig()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(sent, []string{"C", "D", "C"}) {
				t.Fatalf("retry sent=%v, want C D C", sent)
			}
			assertSeen(t, store, "C", true)
		})
	}
}

func TestCallerCancellationDuringFailedSendStopsLaterItems(t *testing.T) {
	t.Parallel()
	for _, earlierFailure := range []bool{false, true} {
		for _, finalItem := range []bool{false, true} {
			t.Run(fmt.Sprintf("earlier=%v/final=%v", earlierFailure, finalItem), func(t *testing.T) {
				store := state.NewMemoryStateStore()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				batch := items("A")
				priorErr := errors.New("earlier send failed")
				cErr := errors.New("C delivery failed without wrapping cancellation")
				var sent []string
				pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
					return batch, nil
				}), store, application.NotificationSenderFunc(func(gotCtx context.Context, n notification.Notification) error {
					if gotCtx != ctx {
						t.Error("wrong caller context")
					}
					sent = append(sent, n.Title)
					if n.Title == "B" {
						return priorErr
					}
					if n.Title == "C" {
						cancel()
						return cErr
					}
					t.Error("D must not be sent after caller cancellation")
					return nil
				}))
				if err := pipeline.RunOnce(ctx, sourceConfig()); err != nil {
					t.Fatal(err)
				}
				batch = nil
				var wantSent []string
				if earlierFailure {
					batch = items("B")
					wantSent = append(wantSent, "B")
				}
				batch = append(batch, items("C")...)
				wantSent = append(wantSent, "C")
				if !finalItem {
					batch = append(batch, items("D")...)
				}
				err := pipeline.RunOnce(ctx, sourceConfig())
				if !errors.Is(err, cErr) || !errors.Is(err, context.Canceled) || (earlierFailure && !errors.Is(err, priorErr)) {
					t.Fatalf("aggregate lost failure/caller cancellation: %v", err)
				}
				if !reflect.DeepEqual(sent, wantSent) {
					t.Fatalf("sent=%v, want %v", sent, wantSent)
				}
				for _, name := range []string{"B", "C", "D"} {
					assertSeen(t, store, name, false)
				}
			})
		}
	}
}

func TestMarkFailureAfterSendAllowsLaterItemsAndRetry(t *testing.T) {
	t.Parallel()
	store := &controlledStore{MemoryStateStore: state.NewMemoryStateStore()}
	ctx := context.WithValue(context.Background(), struct{}{}, "caller")
	batch := items("A")
	var sent, marked []string
	pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
		return batch, nil
	}), store, application.NotificationSenderFunc(func(_ context.Context, n notification.Notification) error {
		assertSeen(t, store, n.Title, false)
		sent = append(sent, n.Title)
		return nil
	}))
	if err := pipeline.RunOnce(ctx, sourceConfig()); err != nil {
		t.Fatal(err)
	}
	markErr := errors.New("state write failed")
	store.mark = func(gotCtx context.Context, id, key string) error {
		if gotCtx != ctx || id != sourceConfig().ID {
			t.Error("mark did not preserve context/configured source")
		}
		marked = append(marked, key)
		if key == "external:C" {
			return markErr
		}
		return store.MemoryStateStore.MarkItemSeen(gotCtx, id, key)
	}
	batch = items("C", "D")
	err := pipeline.RunOnce(ctx, sourceConfig())
	if !errors.Is(err, markErr) || !strings.Contains(err.Error(), `item "external:C": mark seen`) {
		t.Fatalf("error=%v, want mark failure", err)
	}
	if !reflect.DeepEqual(sent, []string{"C", "D"}) || !reflect.DeepEqual(marked, []string{"external:C", "external:D"}) {
		t.Fatalf("sent=%v, marked=%v", sent, marked)
	}
	assertSeen(t, store, "C", false)
	assertSeen(t, store, "D", true)
	store.mark = nil
	if err := pipeline.RunOnce(ctx, sourceConfig()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sent, []string{"C", "D", "C"}) {
		t.Fatalf("expected duplicate C after mark failure: %v", sent)
	}
	assertSeen(t, store, "C", true)
}

func TestCancellationAfterSuccessfulSendDoesNotForceMark(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &controlledStore{MemoryStateStore: state.NewMemoryStateStore()}
	batch := items("A")
	var sent []string
	pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
		return batch, nil
	}), store, application.NotificationSenderFunc(func(_ context.Context, n notification.Notification) error {
		sent = append(sent, n.Title)
		cancel()
		return nil
	}))
	if err := pipeline.RunOnce(ctx, sourceConfig()); err != nil {
		t.Fatal(err)
	}
	store.mark = func(gotCtx context.Context, id, key string) error {
		if gotCtx != ctx || gotCtx.Err() != context.Canceled {
			t.Error("mark used a replacement or uncanceled context")
		}
		return store.MemoryStateStore.MarkItemSeen(gotCtx, id, key)
	}
	batch = items("C", "D")
	if err := pipeline.RunOnce(ctx, sourceConfig()); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want caller cancellation", err)
	}
	if !reflect.DeepEqual(sent, []string{"C"}) {
		t.Fatalf("sent=%v, want only C", sent)
	}
	assertSeen(t, store, "C", false)
	assertSeen(t, store, "D", false)
}

func TestCallerCancellationWithEmptyCollectionOrCollectionError(t *testing.T) {
	t.Parallel()
	for _, collectErr := range []error{nil, errors.New("collection failed")} {
		ctx, cancel := context.WithCancel(context.Background())
		store := state.NewMemoryStateStore()
		pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
			cancel()
			return nil, collectErr
		}), store, application.NotificationSenderFunc(func(context.Context, notification.Notification) error {
			t.Error("canceled collection must not send")
			return nil
		}))
		err := pipeline.RunOnce(ctx, sourceConfig())
		cancel()
		if !errors.Is(err, context.Canceled) || (collectErr != nil && !errors.Is(err, collectErr)) {
			t.Fatalf("error=%v, want caller cancellation and collection error", err)
		}
		if initialized, err := store.IsSourceInitialized(context.Background(), sourceConfig().ID); initialized || err != nil {
			t.Fatalf("canceled collection established baseline: %v, %v", initialized, err)
		}
	}
}

func TestOverlappingRunsRejectedAndGuardReleasedAfterFailure(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"collection", "send"} {
		t.Run(stage, func(t *testing.T) {
			store := state.NewMemoryStateStore()
			var collectCalls, sendCalls atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			blockedErr := errors.New("blocked operation failed")
			blocking := false
			block := func() error {
				close(started)
				select {
				case <-release:
					return blockedErr
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			batch := items("A")
			pipeline := newPipeline(t, collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
				collectCalls.Add(1)
				if blocking && stage == "collection" {
					return nil, block()
				}
				return batch, nil
			}), store, application.NotificationSenderFunc(func(context.Context, notification.Notification) error {
				sendCalls.Add(1)
				if blocking && stage == "send" {
					return block()
				}
				return nil
			}))
			runOK(t, pipeline)
			batch, blocking = items("C"), true
			result := make(chan error, 1)
			go func() { result <- pipeline.RunOnce(ctx, sourceConfig()) }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("first run did not reach controlled operation")
			}
			beforeCollect, beforeSend := collectCalls.Load(), sendCalls.Load()
			if err := pipeline.RunOnce(ctx, sourceConfig()); !errors.Is(err, application.ErrRunInProgress) {
				t.Fatalf("overlap error=%v", err)
			}
			if collectCalls.Load() != beforeCollect || sendCalls.Load() != beforeSend {
				t.Fatal("overlapping call caused collection or sending")
			}
			close(release)
			select {
			case err := <-result:
				if !errors.Is(err, blockedErr) {
					t.Fatalf("first run error=%v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("first run did not return")
			}
			blocking = false // Result channel synchronizes with the completed run.
			runOK(t, pipeline)
			assertSeen(t, store, "C", true)
		})
	}
}
