package state

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"
)

// These wrappers observe the actual driver's transaction/connection boundary.
// The guard prevents reproducing the bug by calling native Rollback on freed TLS.
type lifecycleConnector struct {
	path           string
	txContext      context.Context
	closed         atomic.Bool
	commits        atomic.Int32
	rollbackCause  error
	savepointCause error
	rolledBack     chan bool
}

func (c *lifecycleConnector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c *lifecycleConnector) Connect(context.Context) (driver.Conn, error) {
	raw, err := c.Driver().Open(c.path)
	if err != nil {
		return nil, err
	}
	c.closed.Store(false)
	return &lifecycleConn{Conn: raw, owner: c}, nil
}

type lifecycleConn struct {
	driver.Conn
	owner *lifecycleConnector
}

func (c *lifecycleConn) Close() error { c.owner.closed.Store(true); return c.Conn.Close() }
func (c *lifecycleConn) Prepare(query string) (driver.Stmt, error) {
	if query == "SAVEPOINT signalwatch_cleanup" && c.owner.savepointCause != nil {
		return nil, c.owner.savepointCause
	}
	return c.Conn.Prepare(query)
}
func (c *lifecycleConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.owner.txContext = ctx
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &lifecycleTx{Tx: tx, owner: c.owner}, nil
}

type lifecycleTx struct {
	driver.Tx
	owner *lifecycleConnector
}

func (tx *lifecycleTx) Commit() error { tx.owner.commits.Add(1); return tx.Tx.Commit() }
func (tx *lifecycleTx) Rollback() error {
	closed := tx.owner.closed.Load()
	var err error
	if closed {
		err = errors.New("test guard: native rollback dispatched after Close")
	} else {
		err = tx.Tx.Rollback()
	}
	tx.owner.rolledBack <- closed
	return errors.Join(err, tx.owner.rollbackCause)
}

func observeTransactionLifecycle(t *testing.T, rollbackCause error) (*SQLiteStateStore, *lifecycleConnector) {
	t.Helper()
	dir := t.TempDir()
	s := openTestSQLite(t, dir)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	c := &lifecycleConnector{path: filepath.Join(dir, "state.db"), rollbackCause: rollbackCause, rolledBack: make(chan bool, 1)}
	s.db = sql.OpenDB(c)
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	return s, c
}

func TestSQLiteCancellationAfterFinalStatement(t *testing.T) {
	for _, rollbackCause := range []error{nil, codedError(10)} {
		t.Run(map[bool]string{true: "rollback-failure", false: "normal"}[rollbackCause != nil], func(t *testing.T) {
			s, observer := observeTransactionLifecycle(t, rollbackCause)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conn, tx, err := s.beginWrite(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO seen_items VALUES('s','A')`); err != nil {
				t.Fatal(err)
			}
			cancel()
			// Reproduce the exact already-claimed automatic rollback window when
			// the transaction uses caller cancellation. With private lifetime
			// ownership, finishWrite must perform and observe the rollback itself.
			automatic := observer.txContext.Err() != nil
			if automatic {
				if <-observer.rolledBack {
					t.Fatal("Close before automatic rollback")
				}
			}
			err = finishWrite(ctx, conn, tx, nil)
			if !automatic {
				if <-observer.rolledBack {
					t.Fatal("Close before explicit rollback")
				}
			}
			if !errors.Is(err, context.Canceled) || errors.Is(err, ErrFatal) != (rollbackCause != nil) {
				t.Fatalf("cancellation/fatal precedence: %v", err)
			}
			if rollbackCause != nil && !errors.Is(err, rollbackCause) {
				t.Fatalf("lost rollback cause: %v", err)
			}
			if observer.commits.Load() != 0 {
				t.Fatal("commit started after observed cancellation")
			}
			if got, err := s.HasItem(context.Background(), "s", "A"); got || err != nil {
				t.Fatalf("canceled write persisted: %v %v", got, err)
			}
		})
	}
}

func TestSQLiteCancellationFinishesRollbackBeforeNativeClose(t *testing.T) {
	old := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(old)
	s, observer := observeTransactionLifecycle(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO seen_items VALUES('s','A')`); err != nil {
		t.Fatal(err)
	}
	cancel()
	// With one scheduler thread, Commit sees cancellation before the automatic
	// rollback goroutine runs. Raw must never close before that pending rollback.
	err = finishWrite(ctx, conn, tx, nil)
	if <-observer.rolledBack {
		t.Fatalf("native Close preceded rollback: %v", err)
	}
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrFatal) {
		t.Fatalf("normal cancellation: %v", err)
	}
	if got, err := s.HasItem(context.Background(), "s", "A"); got || err != nil {
		t.Fatalf("canceled write persisted: %v %v", got, err)
	}
}

func TestSQLiteCancellationBeforeStatementDoesNotWrite(t *testing.T) {
	s, observer := observeTransactionLifecycle(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	_, operationErr := tx.ExecContext(ctx, `INSERT INTO seen_items VALUES('s','A')`)
	if operationErr == nil {
		t.Fatal("statement ignored canceled caller")
	}
	err = finishWrite(ctx, conn, tx, operationErr)
	if <-observer.rolledBack {
		t.Fatalf("native Close preceded rollback: %v", err)
	}
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrFatal) {
		t.Fatalf("normal cancellation: %v", err)
	}
	if observer.commits.Load() != 0 {
		t.Fatal("committed canceled statement")
	}
	if got, err := s.HasItem(context.Background(), "s", "A"); got || err != nil {
		t.Fatalf("canceled statement wrote: %v %v", got, err)
	}
}

func TestSQLiteCanceledFinishPreservesIndependentFatalCause(t *testing.T) {
	s, observer := observeTransactionLifecycle(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	cause := codedError(11)
	err = finishWrite(ctx, conn, tx, cause)
	if <-observer.rolledBack {
		t.Fatal("native Close preceded rollback")
	}
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrFatal) || !errors.Is(err, cause) {
		t.Fatalf("lost independent fatal cause: %v", err)
	}
}

type interruptConnector struct {
	sqliteDriver *sqlite.Driver
	path         string
}

func (c *interruptConnector) Driver() driver.Driver { return c.sqliteDriver }
func (c *interruptConnector) Connect(context.Context) (driver.Conn, error) {
	return c.sqliteDriver.Open(c.path)
}

func TestSQLiteCancellationInterruptingWrite(t *testing.T) {
	dir := t.TempDir()
	s := openTestSQLite(t, dir)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &sqlite.Driver{}
	if err := d.RegisterScalarFunction("cancel_write", 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) { cancel(); return int64(1), nil }); err != nil {
		t.Fatal(err)
	}
	s.db = sql.OpenDB(&interruptConnector{sqliteDriver: d, path: filepath.Join(dir, "state.db")})
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO seen_items VALUES('s','prior')`); err != nil {
		t.Fatal(err)
	}
	_, operationErr := tx.ExecContext(ctx, `WITH RECURSIVE seq(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM seq WHERE x<1000000) INSERT INTO seen_items SELECT 's',CAST(x AS TEXT) FROM seq WHERE cancel_write()=1`)
	if operationErr == nil {
		t.Fatal("write did not observe cancellation")
	}
	err = finishWrite(ctx, conn, tx, operationErr)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrFatal) {
		t.Fatalf("interrupted write cleanup: %v", err)
	}
	for _, key := range []string{"prior", "1"} {
		if got, err := s.HasItem(context.Background(), "s", key); got || err != nil {
			t.Fatalf("interrupted write leaked %s: %v %v", key, got, err)
		}
	}
}

func TestSQLiteCleanupControlFailureIsFatal(t *testing.T) {
	s, observer := observeTransactionLifecycle(t, nil)
	cause := codedError(10)
	observer.savepointCause = cause
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO seen_items VALUES('s','A')`); err != nil {
		t.Fatal(err)
	}
	cancel()
	err = finishWrite(ctx, conn, tx, nil)
	if <-observer.rolledBack {
		t.Fatal("native Close preceded rollback after cleanup control failure")
	}
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrFatal) || !errors.Is(err, cause) {
		t.Fatalf("lost cleanup control cause: %v", err)
	}
	if !observer.closed.Load() {
		t.Fatal("cleanup control failure skipped native discard")
	}
	if got, err := s.HasItem(context.Background(), "s", "A"); got || err != nil {
		t.Fatalf("cleanup failure leaked: %v %v", got, err)
	}
}
