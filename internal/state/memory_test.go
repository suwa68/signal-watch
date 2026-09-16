package state

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMemoryStateStoreTracksInitializationIndependently(t *testing.T) {
	t.Parallel()

	store := NewMemoryStateStore()
	initialized, err := store.IsSourceInitialized(context.Background(), "source-a")
	if err != nil {
		t.Fatalf("IsSourceInitialized() error = %v", err)
	}
	if initialized {
		t.Fatal("new source is initialized")
	}

	established, err := store.EstablishBaseline(context.Background(), "source-a", nil)
	if err != nil {
		t.Fatalf("EstablishBaseline() error = %v", err)
	}
	if !established {
		t.Fatal("EstablishBaseline() = false, want true")
	}
	initialized, err = store.IsSourceInitialized(context.Background(), "source-a")
	if err != nil {
		t.Fatalf("IsSourceInitialized() after baseline error = %v", err)
	}
	if !initialized {
		t.Fatal("source is not initialized after baseline")
	}

	otherInitialized, err := store.IsSourceInitialized(context.Background(), "source-b")
	if err != nil {
		t.Fatalf("IsSourceInitialized() other source error = %v", err)
	}
	if otherInitialized {
		t.Fatal("initializing source-a initialized source-b")
	}
}

func TestMemoryStateStoreTracksSeenItemsBySourceAndKey(t *testing.T) {
	t.Parallel()

	store := &MemoryStateStore{}
	seen, err := store.HasItem(context.Background(), "source-a", "key-a")
	if err != nil {
		t.Fatalf("HasItem() error = %v", err)
	}
	if seen {
		t.Fatal("new item is seen")
	}
	if err := store.MarkItemSeen(context.Background(), "source-a", "key-a"); err != nil {
		t.Fatalf("MarkItemSeen() error = %v", err)
	}

	checks := []struct {
		sourceID string
		itemKey  string
		want     bool
	}{
		{sourceID: "source-a", itemKey: "key-a", want: true},
		{sourceID: "source-a", itemKey: "key-b", want: false},
		{sourceID: "source-b", itemKey: "key-a", want: false},
	}
	for _, check := range checks {
		got, err := store.HasItem(context.Background(), check.sourceID, check.itemKey)
		if err != nil {
			t.Fatalf("HasItem(%q, %q) error = %v", check.sourceID, check.itemKey, err)
		}
		if got != check.want {
			t.Errorf("HasItem(%q, %q) = %t, want %t", check.sourceID, check.itemKey, got, check.want)
		}
	}
}

func TestMemoryStateStoreEstablishesBaselineOnlyOnce(t *testing.T) {
	t.Parallel()

	store := NewMemoryStateStore()
	first, err := store.EstablishBaseline(context.Background(), "source", []string{"old-a", "old-b"})
	if err != nil {
		t.Fatalf("EstablishBaseline() first error = %v", err)
	}
	if !first {
		t.Fatal("EstablishBaseline() first = false, want true")
	}
	second, err := store.EstablishBaseline(context.Background(), "source", []string{"new"})
	if err != nil {
		t.Fatalf("EstablishBaseline() second error = %v", err)
	}
	if second {
		t.Fatal("EstablishBaseline() second = true, want false")
	}

	for _, key := range []string{"old-a", "old-b"} {
		seen, err := store.HasItem(context.Background(), "source", key)
		if err != nil || !seen {
			t.Fatalf("HasItem(%q) = %t, %v; want true, nil", key, seen, err)
		}
	}
	seen, err := store.HasItem(context.Background(), "source", "new")
	if err != nil {
		t.Fatalf("HasItem(new) error = %v", err)
	}
	if seen {
		t.Fatal("losing baseline attempt mutated seen items")
	}
}

func TestMemoryStateStoreConcurrentAccess(t *testing.T) {
	t.Parallel()

	store := NewMemoryStateStore()
	const workers = 100
	var establishedCount atomic.Int64
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := 0; index < workers; index++ {
		index := index
		go func() {
			defer wait.Done()
			key := fmt.Sprintf("key-%d", index)
			established, err := store.EstablishBaseline(context.Background(), "baseline", []string{key})
			if err != nil {
				t.Errorf("EstablishBaseline() error = %v", err)
				return
			}
			if established {
				establishedCount.Add(1)
			}
			if err := store.MarkItemSeen(context.Background(), "items", key); err != nil {
				t.Errorf("MarkItemSeen() error = %v", err)
				return
			}
			seen, err := store.HasItem(context.Background(), "items", key)
			if err != nil || !seen {
				t.Errorf("HasItem(%q) = %t, %v; want true, nil", key, seen, err)
			}
			if _, err := store.IsSourceInitialized(context.Background(), "baseline"); err != nil {
				t.Errorf("IsSourceInitialized() error = %v", err)
			}
		}()
	}
	wait.Wait()
	if got := establishedCount.Load(); got != 1 {
		t.Fatalf("successful baseline establishments = %d, want 1", got)
	}
}

func TestMemoryStateStoreValidatesInputBeforeMutation(t *testing.T) {
	t.Parallel()

	store := NewMemoryStateStore()
	if _, err := store.EstablishBaseline(context.Background(), "source", []string{"valid", "  "}); !errors.Is(err, ErrItemKeyRequired) {
		t.Fatalf("EstablishBaseline() error = %v, want ErrItemKeyRequired", err)
	}
	initialized, err := store.IsSourceInitialized(context.Background(), "source")
	if err != nil {
		t.Fatalf("IsSourceInitialized() error = %v", err)
	}
	if initialized {
		t.Fatal("invalid baseline initialized source")
	}
	seen, err := store.HasItem(context.Background(), "source", "valid")
	if err != nil {
		t.Fatalf("HasItem() error = %v", err)
	}
	if seen {
		t.Fatal("invalid baseline partially marked an item seen")
	}
}

func TestMemoryStateStorePropagatesCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := NewMemoryStateStore()
	if _, err := store.EstablishBaseline(ctx, "source", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("EstablishBaseline() error = %v, want context.Canceled", err)
	}
}
