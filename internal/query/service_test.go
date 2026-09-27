package query

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/source"
)

type collectorFunc func(context.Context, config.SourceConfig) ([]source.MonitorItem, error)

func (f collectorFunc) Run(ctx context.Context, cfg config.SourceConfig) ([]source.MonitorItem, error) {
	return f(ctx, cfg)
}

func testDefinition() Definition {
	entryURL := "https://example.com/news"
	return Definition{
		Config:             config.SourceConfig{ID: "news", Name: "News", Type: "html"},
		DefinitionRevision: "revision-1", ConfiguredSourceURL: &entryURL,
	}
}

func TestCollectReturnsEveryCurrentCollectionWithoutState(t *testing.T) {
	t.Parallel()
	var calls int
	service, err := NewService(collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
		calls++
		return []source.MonitorItem{{SourceID: "news", Title: "same", URL: "https://example.com/same"}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		snapshot, err := service.Collect(context.Background(), testDefinition(), DefaultLimits())
		if err != nil || len(snapshot.Items) != 1 || snapshot.Items[0].Title != "same" {
			t.Fatalf("Collect() = %#v, %v", snapshot, err)
		}
	}
	if calls != 2 {
		t.Fatalf("collector calls = %d, want 2", calls)
	}
}

func TestCollectMapsLimitsOptionalFactsAndUTC(t *testing.T) {
	t.Parallel()
	published := time.Date(2026, 9, 27, 16, 0, 0, 123, time.FixedZone("TST", 8*60*60))
	service, _ := NewService(collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
		return []source.MonitorItem{
			{SourceID: "news", ExternalID: "one", Title: "One", Content: "世界🙂abc", PublishedAt: &published},
			{SourceID: "news", Title: "Two", Content: " \t"},
			{SourceID: "news", Title: "Three"},
		}, nil
	}))
	times := []time.Time{time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC), time.Date(2026, 9, 27, 8, 0, 1, 0, time.UTC)}
	service.now = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}
	snapshot, err := service.Collect(context.Background(), testDefinition(), Limits{MaxItems: 2, MaxContentChars: 5})
	if err == nil {
		t.Fatal("expected invalid lower content limit")
	}
	snapshot, err = service.Collect(context.Background(), testDefinition(), Limits{MaxItems: 2, MaxContentChars: 100})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TotalItems != 3 || snapshot.ReturnedItems != 2 || !snapshot.Truncated || len(snapshot.Items) != 2 {
		t.Fatalf("counts = total %d returned %d truncated %v items %d", snapshot.TotalItems, snapshot.ReturnedItems, snapshot.Truncated, len(snapshot.Items))
	}
	if snapshot.Items[0].ContentStatus != "present" || snapshot.Items[0].ContentCompleteness != "unknown" || snapshot.Items[0].PublishedAt == nil || snapshot.Items[0].PublishedAt.Location() != time.UTC {
		t.Fatalf("first item = %#v", snapshot.Items[0])
	}
	if snapshot.Items[1].Content != " \t" || snapshot.Items[1].ContentStatus != "empty" || snapshot.Items[1].PublishedAt != nil {
		t.Fatalf("second item = %#v", snapshot.Items[1])
	}
}

func TestCollectUnicodeTruncationAndEmptyCollection(t *testing.T) {
	t.Parallel()
	long := ""
	for range 101 {
		long += "界"
	}
	batch := []source.MonitorItem{{SourceID: "news", Title: "One", Content: long}}
	service, _ := NewService(collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) { return batch, nil }))
	snapshot, err := service.Collect(context.Background(), testDefinition(), Limits{MaxItems: 20, MaxContentChars: 100})
	if err != nil || len([]rune(snapshot.Items[0].Content)) != 100 || !snapshot.Items[0].ContentTruncated {
		t.Fatalf("truncation = %#v, %v", snapshot, err)
	}
	batch = nil
	snapshot, err = service.Collect(context.Background(), testDefinition(), DefaultLimits())
	if err != nil || snapshot.Items == nil || len(snapshot.Items) != 0 {
		t.Fatalf("empty collection = %#v, %v", snapshot.Items, err)
	}
}

func TestCollectFailsWithoutPartialSuccessAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("adapter failed")
	service, _ := NewService(collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
		return []source.MonitorItem{{Title: "partial"}}, wantErr
	}))
	if got, err := service.Collect(context.Background(), testDefinition(), DefaultLimits()); !errors.Is(err, wantErr) || !reflect.DeepEqual(got, Snapshot{}) {
		t.Fatalf("Collect() = %#v, %v", got, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	service, _ = NewService(collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
		called = true
		return nil, nil
	}))
	if _, err := service.Collect(ctx, testDefinition(), DefaultLimits()); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("pre-canceled Collect() error = %v, called = %v", err, called)
	}

	ctx, cancel = context.WithCancel(context.Background())
	service, _ = NewService(collectorFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
		cancel()
		return []source.MonitorItem{{Title: "late"}}, nil
	}))
	if got, err := service.Collect(ctx, testDefinition(), DefaultLimits()); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, Snapshot{}) {
		t.Fatalf("mid-run canceled Collect() = %#v, %v", got, err)
	}
}
