package monitoring

import (
	"context"
	"errors"
	"testing"

	"github.com/suwa68/signal-watch/internal/itemkey"
	"github.com/suwa68/signal-watch/internal/source"
	"github.com/suwa68/signal-watch/internal/state"
)

func TestProcessorEstablishesBaselineWithoutReturningWork(t *testing.T) {
	t.Parallel()

	store := state.NewMemoryStateStore()
	processor := NewProcessor(store)
	items := []source.MonitorItem{
		{SourceID: "news", ExternalID: "one", Title: "One"},
		{SourceID: "news", URL: "https://example.com/two", Title: "Two"},
	}

	unseen, err := processor.ProcessSuccessfulCollection(context.Background(), "news", items)
	if err != nil {
		t.Fatalf("ProcessSuccessfulCollection() error = %v", err)
	}
	if unseen == nil || len(unseen) != 0 {
		t.Fatalf("ProcessSuccessfulCollection() = %#v, want non-nil empty result", unseen)
	}
	initialized, err := store.IsSourceInitialized(context.Background(), "news")
	if err != nil || !initialized {
		t.Fatalf("IsSourceInitialized() = %t, %v; want true, nil", initialized, err)
	}
	for _, item := range items {
		key, err := itemkey.Build(item)
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		seen, err := store.HasItem(context.Background(), "news", key)
		if err != nil || !seen {
			t.Fatalf("HasItem(%q) = %t, %v; want true, nil", key, seen, err)
		}
	}
}

func TestProcessorEmptyCollectionStillEstablishesBaseline(t *testing.T) {
	t.Parallel()

	store := state.NewMemoryStateStore()
	unseen, err := NewProcessor(store).ProcessSuccessfulCollection(context.Background(), "news", nil)
	if err != nil {
		t.Fatalf("ProcessSuccessfulCollection() error = %v", err)
	}
	if unseen == nil || len(unseen) != 0 {
		t.Fatalf("ProcessSuccessfulCollection() = %#v, want non-nil empty result", unseen)
	}
	initialized, err := store.IsSourceInitialized(context.Background(), "news")
	if err != nil || !initialized {
		t.Fatalf("IsSourceInitialized() = %t, %v; want true, nil", initialized, err)
	}
}

func TestProcessorReturnsOnlyUnseenItemsWithoutMarkingThem(t *testing.T) {
	t.Parallel()

	store := state.NewMemoryStateStore()
	processor := NewProcessor(store)
	oldItem := source.MonitorItem{SourceID: "news", ExternalID: "old", Title: "Old"}
	if _, err := processor.ProcessSuccessfulCollection(context.Background(), "news", []source.MonitorItem{oldItem}); err != nil {
		t.Fatalf("ProcessSuccessfulCollection() baseline error = %v", err)
	}
	newItem := source.MonitorItem{SourceID: "news", ExternalID: "new", Title: "New"}

	unseen, err := processor.ProcessSuccessfulCollection(context.Background(), "news", []source.MonitorItem{oldItem, newItem})
	if err != nil {
		t.Fatalf("ProcessSuccessfulCollection() normal error = %v", err)
	}
	if len(unseen) != 1 || unseen[0].Item != newItem || unseen[0].Key != "external:new" {
		t.Fatalf("ProcessSuccessfulCollection() = %#v, want new item", unseen)
	}
	seen, err := store.HasItem(context.Background(), "news", unseen[0].Key)
	if err != nil {
		t.Fatalf("HasItem() error = %v", err)
	}
	if seen {
		t.Fatal("unseen item was marked seen during discovery")
	}
}

func TestProcessorReturnsOneWorkItemForDuplicateKeysInOneCollection(t *testing.T) {
	t.Parallel()

	store := state.NewMemoryStateStore()
	processor := NewProcessor(store)
	if _, err := processor.ProcessSuccessfulCollection(context.Background(), "news", nil); err != nil {
		t.Fatalf("ProcessSuccessfulCollection() baseline error = %v", err)
	}
	item := source.MonitorItem{SourceID: "news", ExternalID: "duplicate", Title: "First representation"}
	duplicate := source.MonitorItem{SourceID: "news", ExternalID: "duplicate", Title: "Second representation"}

	unseen, err := processor.ProcessSuccessfulCollection(
		context.Background(),
		"news",
		[]source.MonitorItem{item, duplicate},
	)
	if err != nil {
		t.Fatalf("ProcessSuccessfulCollection() error = %v", err)
	}
	if len(unseen) != 1 || unseen[0].Item != item {
		t.Fatalf("ProcessSuccessfulCollection() = %#v, want first representation once", unseen)
	}
}

func TestProcessorRejectsUnidentifiableItemWithoutMutatingBaseline(t *testing.T) {
	t.Parallel()

	store := state.NewMemoryStateStore()
	processor := NewProcessor(store)
	_, err := processor.ProcessSuccessfulCollection(context.Background(), "news", []source.MonitorItem{
		{SourceID: "news", ExternalID: "valid"},
		{SourceID: "news"},
	})
	if !errors.Is(err, itemkey.ErrNoDeterministicIdentity) {
		t.Fatalf("ProcessSuccessfulCollection() error = %v, want ErrNoDeterministicIdentity", err)
	}
	initialized, checkErr := store.IsSourceInitialized(context.Background(), "news")
	if checkErr != nil {
		t.Fatalf("IsSourceInitialized() error = %v", checkErr)
	}
	if initialized {
		t.Fatal("invalid baseline initialized the source")
	}
	seen, checkErr := store.HasItem(context.Background(), "news", "external:valid")
	if checkErr != nil {
		t.Fatalf("HasItem() error = %v", checkErr)
	}
	if seen {
		t.Fatal("invalid baseline partially marked a valid item")
	}
}
