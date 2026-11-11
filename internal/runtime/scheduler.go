// Package runtime supervises independent scheduled source runs.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/state"
)

// Runner completes one source collection and its notification processing.
type Runner interface {
	RunOnce(context.Context, config.SourceConfig) error
}

// ScheduledSource binds a source definition to its independent runner.
type ScheduledSource struct {
	Config config.SourceConfig
	Runner Runner
}

// Run starts each source immediately, then waits its interval after each run
// completes. Runs of one source never overlap or accumulate a backlog.
//
// report, when non-nil, observes every completed run, including successful runs.
// It runs synchronously within each source worker and concurrently across
// sources; it must return promptly and must not own shutdown or external side
// effects. Callers must keep source definitions immutable for Run's lifetime.
//
// Fatal state failures cancel all sources. Run always joins all workers before
// returning, preserving fatal causes even during concurrent caller cancellation.
// Otherwise it returns the governing context error, suitable for exit mapping.
func Run(ctx context.Context, sources []ScheduledSource, report func(config.SourceConfig, error)) error {
	return run(ctx, sources, report, waitFor)
}

func run(ctx context.Context, sources []ScheduledSource, report func(config.SourceConfig, error), wait func(context.Context, time.Duration) bool) error {
	if len(sources) == 0 {
		return errors.New("at least one scheduled source is required")
	}
	intervals := make([]time.Duration, len(sources))
	ids := make(map[string]bool, len(sources))
	for i, source := range sources {
		id := source.Config.ID
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("scheduled source %d: source ID is required", i)
		}
		if ids[id] {
			return fmt.Errorf("duplicate scheduled source ID %q", id)
		}
		ids[id] = true
		if nilRunner(source.Runner) {
			return fmt.Errorf("source %q: runner is required", id)
		}
		seconds := int64(source.Config.IntervalSeconds)
		if seconds <= 0 || seconds > math.MaxInt64/int64(time.Second) {
			return fmt.Errorf("source %q: interval_seconds must be positive and fit time.Duration", id)
		}
		intervals[i] = time.Duration(seconds) * time.Second
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, len(sources))
	for i, source := range sources {
		go func() {
			err := runSource(workerCtx, source, intervals[i], report, wait, cancel)
			if err != nil {
				err = fmt.Errorf("source %q: %w", source.Config.ID, err)
			}
			results <- err
		}()
	}
	var failures []error
	for range sources {
		failures = append(failures, <-results)
	}
	err := errors.Join(failures...)
	if errors.Is(err, state.ErrFatal) {
		return errors.Join(err, ctx.Err())
	}
	return ctx.Err()
}

func runSource(ctx context.Context, source ScheduledSource, interval time.Duration, report func(config.SourceConfig, error), wait func(context.Context, time.Duration) bool, cancel context.CancelFunc) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := source.Runner.RunOnce(ctx, source.Config)
		fatal := errors.Is(err, state.ErrFatal)
		if fatal {
			cancel()
		}
		if report != nil {
			report(source.Config, err)
		}
		if fatal {
			return err
		}
		if ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		}
		if !wait(ctx, interval) {
			return ctx.Err()
		}
	}
}

func waitFor(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func nilRunner(runner Runner) bool {
	if runner == nil {
		return true
	}
	v := reflect.ValueOf(runner)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
