package state

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const initializedSchema = `CREATE TABLE initialized_sources (source_id TEXT NOT NULL PRIMARY KEY)`
const seenSchema = `CREATE TABLE seen_items (source_id TEXT NOT NULL, item_key TEXT NOT NULL, PRIMARY KEY (source_id, item_key))`

// SQLiteStateStore owns a local state directory until Close. It must not be copied.
// SQL connections and the on-disk schema are private infrastructure details.
type SQLiteStateStore struct {
	db        *sql.DB
	ownership *os.File
	lifecycle sync.RWMutex
	closed    bool
	closeErr  error
}

var _ StateStore = (*SQLiteStateStore)(nil)

// OpenSQLite acquires exclusive directory ownership before opening state.db.
// The supported deployment uses a local filesystem with OS advisory locking.
func OpenSQLite(ctx context.Context, directory string) (*SQLiteStateStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !usable(directory) {
		return nil, fmt.Errorf("%w: state directory is required", ErrFatal)
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, classifySQLite(ctx, err)
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, classifySQLite(ctx, err)
	}
	lock, err := acquireOwnership(directory)
	if err != nil {
		return nil, classifySQLite(ctx, err)
	}
	// DSN pragmas are applied by the driver to EVERY newly opened connection.
	params := url.Values{"_pragma": []string{"busy_timeout(1000)", "journal_mode(DELETE)", "synchronous(FULL)"}}
	dsn := (&url.URL{Scheme: "file", Path: filepath.Join(directory, "state.db"), RawQuery: params.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errors.Join(classifySQLite(ctx, err), classifySQLite(ctx, lock.Close()))
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &SQLiteStateStore{db: db, ownership: lock}
	if err := s.initialize(ctx); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	return s, nil
}

// Close waits for store operations, closes SQLite, then releases ownership. The
// stable lock file remains on disk. Repeated Close calls return the same result.
func (s *SQLiteStateStore) Close() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if !s.closed {
		s.closed = true
		s.closeErr = classifySQLite(context.Background(), errors.Join(s.db.Close(), s.ownership.Close()))
	}
	return s.closeErr
}

func (s *SQLiteStateStore) initialize(ctx context.Context) error {
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		return err
	}
	var version int
	err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	var definitions = make(map[string]string)
	if err == nil {
		var rows *sql.Rows
		rows, err = tx.QueryContext(ctx, `SELECT name, sql FROM sqlite_schema WHERE name NOT GLOB 'sqlite_*'`)
		if err == nil {
			for rows.Next() {
				var name, definition string
				if err = rows.Scan(&name, &definition); err != nil {
					break
				}
				definitions[name] = definition
			}
			err = errors.Join(err, rows.Err(), rows.Close())
		}
	}
	if err == nil {
		switch {
		case version == 0 && len(definitions) == 0:
			for _, statement := range []string{initializedSchema, seenSchema, "PRAGMA user_version=1"} {
				if _, err = tx.ExecContext(ctx, statement); err != nil {
					break
				}
			}
		case version == 1 && len(definitions) == 2 && definitions["initialized_sources"] == initializedSchema && definitions["seen_items"] == seenSchema:
		default:
			err = errors.New("unsupported state schema or version")
		}
	}
	if err == nil {
		var integrity string
		err = tx.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity)
		if err == nil && integrity != "ok" {
			err = fmt.Errorf("state integrity check: %s", integrity)
		}
	}
	return finishWrite(ctx, conn, tx, err)
}

func (s *SQLiteStateStore) IsSourceInitialized(ctx context.Context, sourceID string) (bool, error) {
	if err := validateContextAndSource(ctx, sourceID); err != nil {
		return false, err
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM initialized_sources WHERE source_id=?)`, sourceID).Scan(&exists)
	return exists, classifySQLite(ctx, err)
}

func (s *SQLiteStateStore) HasItem(ctx context.Context, sourceID, itemKey string) (bool, error) {
	if err := validateContextSourceAndKey(ctx, sourceID, itemKey); err != nil {
		return false, err
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM seen_items WHERE source_id=? AND item_key=?)`, sourceID, itemKey).Scan(&exists)
	return exists, classifySQLite(ctx, err)
}

func (s *SQLiteStateStore) EstablishBaseline(ctx context.Context, sourceID string, itemKeys []string) (bool, error) {
	if err := validateContextAndSource(ctx, sourceID); err != nil {
		return false, err
	}
	for _, key := range itemKeys {
		if !usable(key) {
			return false, ErrItemKeyRequired
		}
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		return false, err
	}
	var initialized bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM initialized_sources WHERE source_id=?)`, sourceID).Scan(&initialized)
	if err == nil && !initialized {
		for _, key := range itemKeys {
			if _, err = tx.ExecContext(ctx, `INSERT INTO seen_items(source_id,item_key) VALUES(?,?) ON CONFLICT DO NOTHING`, sourceID, key); err != nil {
				break
			}
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO initialized_sources(source_id) VALUES(?)`, sourceID)
		}
	}
	err = finishWrite(ctx, conn, tx, err)
	return !initialized && err == nil, err
}

func (s *SQLiteStateStore) MarkItemSeen(ctx context.Context, sourceID, itemKey string) error {
	if err := validateContextSourceAndKey(ctx, sourceID, itemKey); err != nil {
		return err
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	conn, tx, err := s.beginWrite(ctx)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO seen_items(source_id,item_key) VALUES(?,?) ON CONFLICT DO NOTHING`, sourceID, itemKey)
	return finishWrite(ctx, conn, tx, err)
}

func (s *SQLiteStateStore) beginWrite(ctx context.Context) (*sql.Conn, *sql.Tx, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, nil, classifySQLite(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		conn.Close()
		return nil, nil, err
	}
	// Only finishWrite owns transaction termination. A caller-canceled BeginTx
	// context would let database/sql's asynchronous rollback race with discard;
	// ErrTxDone does not mean that native rollback has finished. Acquisition and
	// all normal SQL still use ctx, and finishWrite checks it before commit.
	// BEGIN/COMMIT themselves use the driver's bounded busy timeout.
	tx, err := conn.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		cleanup := discardConnection(conn)
		conn.Close()
		if cleanup != nil {
			return nil, nil, errors.Join(ErrFatal, err, cleanup, ctx.Err())
		}
		return nil, nil, classifySQLite(ctx, err)
	}
	return conn, tx, nil
}

func finishWrite(ctx context.Context, conn *sql.Conn, tx *sql.Tx, operationErr error) error {
	defer conn.Close()
	// Never begin a commit after observing caller cancellation, including when
	// its final statement succeeded. Preserve independent backend failures.
	operationErr = errors.Join(operationErr, ctx.Err())
	var cleanup error
	if operationErr == nil {
		operationErr = tx.Commit()
	} else {
		// SQLite itself can roll back an interrupted write while sql.Tx remains
		// live. A SAVEPOINT nests in an active transaction or starts an empty
		// deferred transaction, so plain ROLLBACK is valid in either state. This
		// is cleanup control only: no data writes, RELEASE, or COMMIT. Every
		// cleanup error is retained, and rollback/discard still run on failure.
		_, cleanup = tx.ExecContext(context.WithoutCancel(ctx), "SAVEPOINT signalwatch_cleanup")
		cleanup = errors.Join(cleanup, tx.Rollback())
	}
	if operationErr == nil {
		return nil
	}
	// modernc v1.59 rolls back a failed COMMIT but hides rollback errors. Keep the
	// sql.Conn pinned until a synchronous driver Close has discarded any remaining
	// native transaction. Commit/Rollback above completes synchronously: the
	// private transaction context prevents a competing automatic rollback. Raw
	// alone would only serialize individual calls, not complete the transaction.
	// Raw is used only for exclusive cleanup, never for SQL.
	cleanup = errors.Join(cleanup, discardConnection(conn))
	if cleanup != nil {
		return errors.Join(ErrFatal, operationErr, cleanup, ctx.Err())
	}
	return classifySQLite(ctx, operationErr)
}

func discardConnection(conn *sql.Conn) error {
	var closeErr error
	rawErr := conn.Raw(func(raw any) error {
		closeErr = raw.(driver.Conn).Close()
		// Tell database/sql never to reuse this connection. The pinned modernc
		// driver's Close is idempotent, including the pool's subsequent Close.
		return driver.ErrBadConn
	})
	if errors.Is(rawErr, driver.ErrBadConn) || errors.Is(rawErr, sql.ErrConnDone) {
		rawErr = nil
	}
	return errors.Join(closeErr, rawErr)
}
