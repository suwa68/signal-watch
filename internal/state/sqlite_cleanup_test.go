package state

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"testing"

	"modernc.org/sqlite"
)

// Only cleanup failure is injected; the transaction, connection lifecycle,
// rollback on close, and database/sql pool behavior use the actual driver.
type cleanupFaultConnector struct {
	path   string
	cause  error
	opened int
	closed int
}

func (c *cleanupFaultConnector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c *cleanupFaultConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.Driver().Open(c.path)
	if err != nil {
		return nil, err
	}
	c.opened++
	return &cleanupFaultConn{Conn: conn, owner: c}, nil
}

type cleanupFaultConn struct {
	driver.Conn
	owner *cleanupFaultConnector
}

func (c *cleanupFaultConn) Close() error {
	c.owner.closed++
	return errors.Join(c.Conn.Close(), c.owner.cause)
}

func TestSQLiteCleanupFailureRetainsBothCausesAndDiscards(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTestSQLite(t, dir)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	closeCause := errors.New("injected close failure")
	connector := &cleanupFaultConnector{path: filepath.Join(dir, "state.db"), cause: closeCause}
	s.db = sql.OpenDB(connector)
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO seen_items VALUES('s','uncommitted')`); err != nil {
		t.Fatal(err)
	}
	busyCause := codedError(5)
	err = finishWrite(ctx, conn, tx, busyCause)
	if !errors.Is(err, ErrFatal) || errors.Is(err, ErrUnavailable) || !errors.Is(err, busyCause) || !errors.Is(err, closeCause) {
		t.Fatalf("lost cleanup causes: %v", err)
	}
	if connector.closed != 2 {
		t.Fatalf("explicit and pool Close calls: %d", connector.closed)
	}
	connector.cause = nil
	if got, err := s.HasItem(ctx, "s", "uncommitted"); got || err != nil {
		t.Fatalf("reused unsafe connection: %v %v", got, err)
	}
	if connector.opened != 2 {
		t.Fatalf("connection wasn't replaced: %d", connector.opened)
	}
}

func TestSQLiteDiscardAlreadyClosedOrBadConnectionIsSafe(t *testing.T) {
	s := openTestSQLite(t, t.TempDir())
	ctx := context.Background()
	for _, mode := range []string{"closed", "bad", "explicit"} {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "closed":
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
		case "bad":
			if err := conn.Raw(func(any) error { return driver.ErrBadConn }); !errors.Is(err, driver.ErrBadConn) {
				t.Fatal(err)
			}
		}
		if err := discardConnection(conn); err != nil {
			t.Fatalf("%s discard: %v", mode, err)
		}
		conn.Close()
		if err := s.MarkItemSeen(ctx, "s", mode); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSQLiteActualCommitBusyWithCleanupFailureIsFatal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTestSQLite(t, dir)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	closeCause := errors.New("injected close failure after COMMIT BUSY")
	connector := &cleanupFaultConnector{path: filepath.Join(dir, "state.db"), cause: closeCause}
	s.db = sql.OpenDB(connector)
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
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
	var count int
	if err := tx.QueryRow("SELECT count(*) FROM seen_items").Scan(&count); err != nil {
		t.Fatal(err)
	}
	_, err = s.EstablishBaseline(ctx, "s", []string{"A", "B"})
	var sqliteCause *sqlite.Error
	if !errors.Is(err, ErrFatal) || errors.Is(err, ErrUnavailable) || !errors.Is(err, closeCause) || !errors.As(err, &sqliteCause) || sqliteCause.Code()&255 != 5 {
		t.Fatalf("real busy and cleanup causes: %v", err)
	}
	connector.cause = nil
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got, err := s.HasItem(ctx, "s", "A"); got || err != nil {
		t.Fatalf("failed cleanup leaked item: %v %v", got, err)
	}
	if connector.opened != 2 {
		t.Fatalf("failed connection reused: %d", connector.opened)
	}
}

func TestSQLiteCancellationDuringTransactionCleansUp(t *testing.T) {
	s := openTestSQLite(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO seen_items VALUES('s','uncommitted')`); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := finishWrite(ctx, conn, tx, ctx.Err()); !errors.Is(err, context.Canceled) || errors.Is(err, ErrFatal) {
		t.Fatalf("cancel cleanup: %v", err)
	}
	if got, err := s.HasItem(context.Background(), "s", "uncommitted"); got || err != nil {
		t.Fatalf("canceled key visible: %v %v", got, err)
	}
	if err := s.MarkItemSeen(context.Background(), "s", "new"); err != nil {
		t.Fatal(err)
	}
}
