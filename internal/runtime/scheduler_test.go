package runtime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/state"
)

type runnerFunc func(context.Context, config.SourceConfig) error

func (f runnerFunc) RunOnce(ctx context.Context, cfg config.SourceConfig) error { return f(ctx, cfg) }

type waitRequest struct {
	delay time.Duration
	fire  chan struct{}
	done  chan struct{}
}

func fakeWait(requests chan<- waitRequest) func(context.Context, time.Duration) bool {
	return func(ctx context.Context, delay time.Duration) bool {
		r := waitRequest{delay: delay, fire: make(chan struct{}), done: make(chan struct{})}
		defer close(r.done)
		select {
		case requests <- r:
		case <-ctx.Done():
			return false
		}
		select {
		case <-r.fire:
			return true
		case <-ctx.Done():
			return false
		}
	}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for controlled scheduler event")
		var zero T
		return zero
	}
}

func scheduled(id string, seconds int, runner Runner) ScheduledSource {
	return ScheduledSource{Config: config.SourceConfig{ID: id, Name: id, IntervalSeconds: seconds}, Runner: runner}
}

// Missing validation would start the first runner before rejecting later input.
func TestRunValidatesEverySourceBeforeStarting(t *testing.T) {
	var calls atomic.Int32
	runner := runnerFunc(func(context.Context, config.SourceConfig) error {
		calls.Add(1)
		return state.ErrFatal // A validation regression terminates instead of hanging.
	})
	for _, tc := range []struct {
		name string
		bad  ScheduledSource
	}{
		{"empty ID", scheduled("", 1, runner)},
		{"blank ID", scheduled(" \t", 1, runner)},
		{"duplicate ID", scheduled("valid", 1, runner)},
		{"zero interval", scheduled("bad", 0, runner)},
		{"negative interval", scheduled("bad", -1, runner)},
		{"overflow interval", scheduled("bad", int(math.MaxInt64/int64(time.Second))+1, runner)},
		{"nil runner", scheduled("bad", 1, nil)},
		{"typed nil runner", scheduled("bad", 1, runnerFunc(nil))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := Run(ctx, []ScheduledSource{scheduled("valid", 1, runner), tc.bad}, nil)
			if err == nil || errors.Is(err, context.Canceled) {
				t.Fatalf("invalid input error = %v", err)
			}
			if calls.Load() != 0 {
				t.Fatal("validation started a runner")
			}
		})
	}
}

// A ticker, shared loop, pre-run delay, or overlapping run breaks these barriers.
func TestRunImmediateIndependentCompletionBasedDelays(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan string, 8)
	releaseA := make(chan struct{})
	var activeA atomic.Int32
	requests := make(chan waitRequest, 8)
	runner := runnerFunc(func(ctx context.Context, cfg config.SourceConfig) error {
		started <- cfg.ID
		if cfg.ID == "A" {
			if activeA.Add(1) != 1 {
				t.Error("same source overlapped")
			}
			defer activeA.Add(-1)
			select {
			case <-releaseA:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	reports := make(chan string, 8)
	result := make(chan error, 1)
	go func() {
		result <- run(ctx, []ScheduledSource{scheduled("A", 3600, runner), scheduled("B", 7, runner)}, func(cfg config.SourceConfig, err error) {
			if err != nil {
				t.Errorf("run %s: %v", cfg.ID, err)
			}
			reports <- cfg.ID
		}, fakeWait(requests))
	}()
	first, second := receive(t, started), receive(t, started)
	if first == second {
		t.Fatal("both sources must start immediately")
	}
	if got := receive(t, reports); got != "B" {
		t.Fatalf("report = %s, want B while A blocked", got)
	}
	waitB := receive(t, requests)
	if waitB.delay != 7*time.Second {
		t.Fatalf("B delay = %v", waitB.delay)
	}
	for range 2 {
		close(waitB.fire)
		if got := receive(t, started); got != "B" {
			t.Fatalf("started = %s, want B", got)
		}
		receive(t, reports)
		waitB = receive(t, requests)
		if waitB.delay != 7*time.Second {
			t.Fatalf("B delay = %v", waitB.delay)
		}
	}
	close(releaseA)
	if got := receive(t, reports); got != "A" {
		t.Fatalf("completed = %s", got)
	}
	waitA := receive(t, requests)
	if waitA.delay != time.Hour {
		t.Fatalf("A delay = %v", waitA.delay)
	}
	select {
	case got := <-started:
		t.Fatalf("source caught up without completion delay: %s", got)
	default:
	}
	close(waitA.fire)
	if got := receive(t, started); got != "A" {
		t.Fatalf("started = %s", got)
	}
	receive(t, reports)
	waitA = receive(t, requests)
	cancel()
	if err := receive(t, result); err != context.Canceled {
		t.Fatalf("stop = %v", err)
	}
	// Run must join every pending wait before allowing resource owners to close.
	for _, request := range []waitRequest{waitA, waitB} {
		select {
		case <-request.done:
		default:
			t.Fatal("Run returned before wait exited")
		}
	}
}

// Error identity alone must never substitute for checking the governing ctx.
func TestRunContinuesAfterRecoverableErrors(t *testing.T) {
	for _, cause := range []error{errors.New("collection failed"), context.Canceled, context.DeadlineExceeded, state.ErrUnavailable} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests := make(chan waitRequest, 1)
			reports := make(chan error, 2)
			calls := 0
			runner := runnerFunc(func(context.Context, config.SourceConfig) error {
				calls++
				if calls == 1 {
					return fmt.Errorf("source operation: %w", cause)
				}
				return nil
			})
			result := make(chan error, 1)
			go func() {
				result <- run(ctx, []ScheduledSource{scheduled("news", 11, runner)}, func(_ config.SourceConfig, err error) { reports <- err }, fakeWait(requests))
			}()
			if err := receive(t, reports); !errors.Is(err, cause) {
				t.Fatalf("report lost cause: %v", err)
			}
			request := receive(t, requests)
			if request.delay != 11*time.Second {
				t.Fatalf("delay = %v", request.delay)
			}
			close(request.fire)
			if err := receive(t, reports); err != nil {
				t.Fatalf("successful run report = %v", err)
			}
			receive(t, requests)
			cancel()
			if err := receive(t, result); err != context.Canceled {
				t.Fatalf("stop = %v", err)
			}
		})
	}
}

// A first cancellation result must not hide a later completed fatal result.
func TestRunFatalWinsAndJoinsAllWorkers(t *testing.T) {
	for _, externalCancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("external_cancel=%v", externalCancel), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{}, 2)
			releaseFatal, releaseSibling := make(chan struct{}), make(chan struct{})
			siblingCanceled := make(chan struct{})
			cause := errors.New("disk failed")
			fatal := fmt.Errorf("mark: %w", errors.Join(state.ErrFatal, cause, context.DeadlineExceeded))
			reports := make(chan error, 2)
			sources := []ScheduledSource{
				scheduled("fatal", 1, runnerFunc(func(context.Context, config.SourceConfig) error { started <- struct{}{}; <-releaseFatal; return fatal })),
				scheduled("sibling", 1, runnerFunc(func(ctx context.Context, _ config.SourceConfig) error {
					started <- struct{}{}
					<-ctx.Done()
					close(siblingCanceled)
					<-releaseSibling
					return ctx.Err()
				})),
			}
			result := make(chan error, 1)
			go func() { result <- Run(ctx, sources, func(_ config.SourceConfig, err error) { reports <- err }) }()
			receive(t, started)
			receive(t, started)
			if externalCancel {
				cancel()
				receive(t, siblingCanceled)
				close(releaseSibling)
				if err := receive(t, reports); !errors.Is(err, context.Canceled) {
					t.Fatalf("sibling report = %v", err)
				}
				select {
				case err := <-result:
					t.Fatalf("returned before fatal runner joined: %v", err)
				default:
				}
			}
			close(releaseFatal)
			if !externalCancel {
				receive(t, siblingCanceled)
				select {
				case err := <-result:
					t.Fatalf("returned before sibling joined: %v", err)
				default:
				}
				close(releaseSibling)
			}
			err := receive(t, result)
			for _, want := range []error{state.ErrFatal, cause, context.DeadlineExceeded, context.Canceled} {
				if !errors.Is(err, want) {
					t.Errorf("aggregate %v lost %v", err, want)
				}
			}
		})
	}
}

func TestRunAlreadyCanceledContextNeverCallsRunner(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		var ctx context.Context
		var cancel context.CancelFunc
		want := context.Canceled
		if deadline {
			ctx, cancel = context.WithDeadline(context.Background(), time.Unix(1, 0))
			want = context.DeadlineExceeded
		} else {
			ctx, cancel = context.WithCancel(context.Background())
			cancel()
		}
		err := Run(ctx, []ScheduledSource{scheduled("news", 1, runnerFunc(func(context.Context, config.SourceConfig) error { t.Error("canceled runner called"); return nil }))}, nil)
		cancel()
		if err != want {
			t.Fatalf("stop = %v, want %v", err, want)
		}
	}
}

func TestRunRejectsEmptySources(t *testing.T) {
	if err := Run(context.Background(), nil, nil); err == nil {
		t.Fatal("empty sources accepted")
	}
}
