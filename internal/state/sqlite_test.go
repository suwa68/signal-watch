package state

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func openTestSQLite(t *testing.T, dir string) *SQLiteStateStore {
	t.Helper()
	s, err := OpenSQLite(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func TestSQLitePersistenceAndBaselineContract(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTestSQLite(t, dir)
	if err := s.MarkItemSeen(ctx, "source", "prior"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.IsSourceInitialized(ctx, "source"); got || err != nil {
		t.Fatalf("pre-init mark: %v %v", got, err)
	}
	if got, err := s.EstablishBaseline(ctx, "source", []string{"A", "B", "A"}); !got || err != nil {
		t.Fatalf("baseline: %v %v", got, err)
	}
	if got, err := s.EstablishBaseline(ctx, "source", []string{"loser"}); got || err != nil {
		t.Fatalf("losing baseline: %v %v", got, err)
	}
	if got, err := s.EstablishBaseline(ctx, "empty", nil); !got || err != nil {
		t.Fatalf("empty: %v %v", got, err)
	}
	if err := s.MarkItemSeen(ctx, "source", "B"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestSQLite(t, dir)
	for _, source := range []string{"source", "empty"} {
		if got, err := s.IsSourceInitialized(ctx, source); !got || err != nil {
			t.Fatalf("initialized %s: %v %v", source, got, err)
		}
	}
	for _, key := range []string{"prior", "A", "B"} {
		if got, err := s.HasItem(ctx, "source", key); !got || err != nil {
			t.Fatalf("persisted %s: %v %v", key, got, err)
		}
	}
	for _, pair := range [][2]string{{"source", "loser"}, {"other", "A"}, {"empty", "A"}} {
		if got, err := s.HasItem(ctx, pair[0], pair[1]); got || err != nil {
			t.Fatalf("unexpected item: %v %v", got, err)
		}
	}
}

func TestSQLiteValidationAndCancellation(t *testing.T) {
	s := openTestSQLite(t, t.TempDir())
	ctx := context.Background()
	if _, err := s.EstablishBaseline(ctx, "bad", []string{"A", " "}); !errors.Is(err, ErrItemKeyRequired) {
		t.Fatal(err)
	}
	if got, err := s.HasItem(ctx, "bad", "A"); got || err != nil {
		t.Fatalf("partial baseline: %v %v", got, err)
	}
	if _, err := s.IsSourceInitialized(ctx, " "); !errors.Is(err, ErrSourceIDRequired) {
		t.Fatal(err)
	}
	if err := s.MarkItemSeen(ctx, "s", ""); !errors.Is(err, ErrItemKeyRequired) {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, call := range []func() error{
		func() error { _, e := s.IsSourceInitialized(canceled, "s"); return e },
		func() error { _, e := s.HasItem(canceled, "s", "A"); return e },
		func() error { _, e := s.EstablishBaseline(canceled, "s", nil); return e },
		func() error { return s.MarkItemSeen(canceled, "s", "A") },
	} {
		if err := call(); !errors.Is(err, context.Canceled) || errors.Is(err, ErrFatal) {
			t.Fatal(err)
		}
	}
}

func TestSQLiteConcurrentInitializationAndMarks(t *testing.T) {
	s := openTestSQLite(t, t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var wins atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			won, err := s.EstablishBaseline(ctx, "same", []string{fmt.Sprint("baseline-", i)})
			if err != nil {
				t.Error(err)
			}
			if won {
				wins.Add(1)
			}
			if err := s.MarkItemSeen(ctx, "same", fmt.Sprint("mark-", i)); err != nil {
				t.Error(err)
			}
			if _, err := s.EstablishBaseline(ctx, fmt.Sprint("source-", i), nil); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("winners = %d", wins.Load())
	}
	var baselines int
	for i := 0; i < 20; i++ {
		if got, err := s.HasItem(ctx, "same", fmt.Sprint("mark-", i)); !got || err != nil {
			t.Fatalf("lost mark: %v %v", got, err)
		}
		if got, err := s.HasItem(ctx, "same", fmt.Sprint("baseline-", i)); err != nil {
			t.Fatal(err)
		} else if got {
			baselines++
		}
	}
	if baselines != 1 {
		t.Fatalf("losing baseline leaked: %d", baselines)
	}
}
