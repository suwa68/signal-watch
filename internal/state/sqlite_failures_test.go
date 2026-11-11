package state

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type codedError int

func (e codedError) Error() string { return "injected SQLite failure" }
func (e codedError) Code() int     { return int(e) }

func TestSQLiteErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		code int
		want error
	}{{5, ErrUnavailable}, {5 | 256, ErrUnavailable}, {6, ErrFatal}, {10, ErrFatal}, {13, ErrFatal}, {11, ErrFatal}, {26, ErrFatal}, {8, ErrFatal}, {14, ErrFatal}, {1, ErrFatal}, {9, ErrFatal}, {999, ErrFatal}} {
		cause := codedError(tc.code)
		err := classifySQLite(context.Background(), cause)
		if !errors.Is(err, tc.want) || !errors.Is(err, cause) {
			t.Fatalf("code %d: %v", tc.code, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, cause := range []error{context.Canceled, codedError(9)} {
		err := classifySQLite(ctx, cause)
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrFatal) {
			t.Fatalf("cancellation: %v", err)
		}
	}
	for _, cause := range []error{codedError(10), errors.New("unknown"), errors.Join(context.Canceled, errors.New("unknown")), errors.Join(codedError(5), codedError(10))} {
		err := classifySQLite(ctx, cause)
		if !errors.Is(err, ErrFatal) || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Fatalf("simultaneous fatal/context: %v", err)
		}
	}
}

func TestSQLiteFailedInsertIsAtomic(t *testing.T) {
	s := openTestSQLite(t, t.TempDir())
	ctx := context.Background()
	if _, err := s.db.Exec(`CREATE TRIGGER fail_insert BEFORE INSERT ON seen_items WHEN NEW.item_key='B' BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EstablishBaseline(ctx, "s", []string{"A", "B"}); !errors.Is(err, ErrFatal) {
		t.Fatal(err)
	}
	if got, err := s.HasItem(ctx, "s", "A"); got || err != nil {
		t.Fatalf("partial key: %v %v", got, err)
	}
	if got, err := s.IsSourceInitialized(ctx, "s"); got || err != nil {
		t.Fatalf("partial marker: %v %v", got, err)
	}
}

func TestSQLiteCommitBusyDiscardsAndRecovers(t *testing.T) {
	dir := t.TempDir()
	s := openTestSQLite(t, dir)
	ctx := context.Background()
	reader, err := sql.Open("sqlite", filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	tx, err := reader.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM seen_items`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EstablishBaseline(ctx, "s", []string{"A", "B"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("commit should hit real BUSY: %v", err)
	}
	// Even before the blocking reader releases, the failed writer's keys must be absent.
	if got, err := s.HasItem(ctx, "s", "A"); got || err != nil {
		t.Fatalf("uncommitted key visible: %v %v", got, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got, err := s.IsSourceInitialized(ctx, "s"); got || err != nil {
		t.Fatalf("failed marker: %v %v", got, err)
	}
	if err := s.MarkItemSeen(ctx, "s", "C"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestSQLite(t, dir)
	for _, key := range []string{"A", "B"} {
		if got, err := s.HasItem(ctx, "s", key); got || err != nil {
			t.Fatalf("reopened failed key: %v %v", got, err)
		}
	}
	if got, err := s.HasItem(ctx, "s", "C"); !got || err != nil {
		t.Fatalf("recovery write missing: %v %v", got, err)
	}
}

func TestSQLiteReplacementConnectionSettingsAndCanceledWaiter(t *testing.T) {
	s := openTestSQLite(t, t.TempDir())
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	waiter, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := s.MarkItemSeen(waiter, "s", "A"); !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrFatal) {
		t.Fatalf("waiter: %v", err)
	}
	if err := conn.Raw(func(any) error { return driver.ErrBadConn }); !errors.Is(err, driver.ErrBadConn) {
		t.Fatal(err)
	}
	conn.Close()
	var journal string
	var sync, busy int
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if journal != "delete" || sync != 2 || busy != 1000 {
		t.Fatalf("replacement settings: %s %d %d", journal, sync, busy)
	}
	if s.db.Stats().MaxOpenConnections != 1 {
		t.Fatal("unbounded connection pool")
	}
}

func TestSQLiteRejectsCorruptAndUnexpectedSchemaWithoutReset(t *testing.T) {
	for _, kind := range []string{"corrupt", "unexpected", "version", "trigger", "missing", "prefix"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.db")
			if kind == "corrupt" {
				if err := os.WriteFile(path, []byte("not a database"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				s := openTestSQLite(t, dir)
				q := map[string]string{"unexpected": "CREATE TABLE foreign_table(x)", "version": "PRAGMA user_version=99", "trigger": "CREATE TRIGGER extra AFTER INSERT ON seen_items BEGIN SELECT 1; END", "missing": "DROP TABLE seen_items", "prefix": "CREATE TABLE sqliteXshadow(x)"}[kind]
				if _, err := s.db.Exec(q); err != nil {
					t.Fatal(err)
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if s, err := OpenSQLite(context.Background(), dir); !errors.Is(err, ErrFatal) {
				if s != nil {
					s.Close()
				}
				t.Fatalf("accepted %s: %v", kind, err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("rejected state modified")
			}
			// Failure must release the ownership lock.
			lock, err := acquireOwnership(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := lock.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
