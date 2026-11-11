package state

import (
	"context"
	"errors"
)

var (
	// ErrUnavailable permits a later scheduled attempt after safe cleanup.
	ErrUnavailable = errors.New("state temporarily unavailable")
	// ErrFatal signals that durable state cannot safely support continued monitoring.
	ErrFatal = errors.New("fatal state failure")
)

// classifySQLite uses primary result codes, including extended SQLite codes.
// Callers must establish safe cleanup before classifying BUSY as unavailable.
func classifySQLite(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(sqliteCategory(ctx, err), err, ctx.Err())
}

// Inspect every joined cause so a cancellation or BUSY cannot hide a second,
// fatal fault (for example a row read followed by a failed row close).
func sqliteCategory(ctx context.Context, err error) error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var category error
		for _, cause := range joined.Unwrap() {
			switch sqliteCategory(ctx, cause) {
			case ErrFatal:
				return ErrFatal
			case ErrUnavailable:
				category = ErrUnavailable
			}
		}
		return category
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return sqliteCategory(ctx, wrapped.Unwrap())
	}
	if code, ok := err.(interface{ Code() int }); ok {
		switch code.Code() & 0xff {
		case 5: // SQLITE_BUSY
			return ErrUnavailable
		case 9: // SQLITE_INTERRUPT is cancellation only with a governing context.
			if ctx.Err() != nil {
				return nil
			}
		}
	} else if (err == context.Canceled || err == context.DeadlineExceeded) && ctx.Err() != nil {
		return nil
	}
	// LOCKED, hard storage faults, and unknown failures are all fatal. A context
	// canceled concurrently must never erase the backend fault.
	return ErrFatal
}
