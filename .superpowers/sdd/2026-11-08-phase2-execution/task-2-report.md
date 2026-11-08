# Task 2 implementer report

Status: DONE, awaiting Parent's independent review. Implemented Task 2 only on
`feat/phase2-monitoring-mvp`, starting from
`dc0bcbd3ccee329366a049d2bb788f7e473c8daf` (reviewed durable state at `202688a`).
Issue: https://github.com/suwa68/signal-watch/issues/31.
Existing PR: https://github.com/suwa68/signal-watch/pull/32.

## Owned files and public contract

- `internal/runtime/scheduler.go`: scheduler, validation, worker supervision,
  private wait seam, production timer cleanup.
- `internal/runtime/scheduler_test.go`: deterministic wait/channel barriers and
  validation/error supervision tests.
- `internal/runtime/scheduler_pipeline_test.go`: real Pipeline integration proving
  delayed discovery of a fatal mark error while a later item remains blocked.
- This report.

The public API is exactly:

```go
type Runner interface {
    RunOnce(context.Context, config.SourceConfig) error
}
type ScheduledSource struct {
    Config config.SourceConfig
    Runner Runner
}
func Run(ctx context.Context, sources []ScheduledSource,
    report func(config.SourceConfig, error)) error
```

`Run` validates the entire input before creating goroutines: nonempty source
list, nonblank IDs, unique IDs, nonnil runners including typed nil interfaces,
and positive interval seconds representable as `time.Duration`. It starts one
sequential worker per source immediately. Every worker waits the configured
interval after its completed run/report and never catches up or overlaps itself.

The optional callback observes every completed RunOnce, including nil error.
Callbacks run synchronously in their worker and concurrently across sources;
they must return promptly and must not own shutdown or external side effects.
No global callback lock exists. Callers keep definitions immutable during Run.

## Root causes, decisions, and scope

There was no scheduler package at the base revision. The central correctness
distinction is a returned operation error versus the actual governing context:
sender-local `context.Canceled` and `context.DeadlineExceeded`, ordinary failures,
and `state.ErrUnavailable` are reportable recoverable runs while the actual
worker context remains active. Error identity alone never stops a source.

Any returned error tree containing `state.ErrFatal` cancels the shared derived
context before reporting that completed run. Every worker result is gathered
before return. Fatal aggregates retain wrapped/joined original errors and win
over concurrent caller cancellation, including when a sibling reports its
cancellation first. Without fatal results Run returns the original governing
context error directly, allowing command-level signal exit mapping.

Parent clarified that empty-source validation includes a zero-length slice,
so Run rejects it; assembly will independently reject an empty catalog later.
Parent confirmed nil callbacks and synchronous/concurrent per-worker reporting.
These are clarifications, not scope expansions. No Task 1 state internals,
Pipeline behavior, command, configuration, notifier, OS signal, store ownership,
or Task 3 implementation changed. No subagents were spawned, live sources used,
Telegram sent, secrets read/committed, or push/merge/deployment performed.

## RED evidence

All test bodies preceded the implementation. The first compile smoke with only
tests used:

```sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./internal/runtime
```

It exited 1 with missing `Runner`, `ScheduledSource`, `Run`, and private `run`.
This established the missing API, not behavioral RED. An API-only skeleton
returning nil was then written, and the following command exited 1 with actual
assertion failures:

```sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./internal/runtime -run 'TestRunValidates|TestRunAlreadyCanceled|TestRunRejectsEmpty' -count=1
```

Output excerpts preserved from the tool transcript:

```text
--- FAIL: TestRunValidatesEverySourceBeforeStarting (0.00s)
    --- FAIL: TestRunValidatesEverySourceBeforeStarting/empty_ID (0.00s)
        scheduler_test.go:82: invalid input error = <nil>
```

The blank ID, duplicate ID, zero interval, negative interval, overflow interval,
nil runner and typed nil runner subtests all failed with the same missing
validation behavior. Remaining direct assertions:

```text
--- FAIL: TestRunAlreadyCanceledContextNeverCallsRunner (0.00s)
    scheduler_test.go:223: stop = <nil>, want context canceled
--- FAIL: TestRunRejectsEmptySources (0.00s)
    scheduler_test.go:228: empty sources accepted
FAIL github.com/suwa68/signal-watch/internal/runtime 0.005s
```

After adding the real Pipeline delayed-fatal test, the complete behavioral RED
command was:

```sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./internal/runtime -count=1
```

Exit 1; output included:

```text
--- FAIL: TestRunDiscoversPipelineFatalOnlyAfterLaterItemCompletes (5.01s)
    scheduler_pipeline_test.go:66: timed out waiting for controlled scheduler event
--- FAIL: TestRunImmediateIndependentCompletionBasedDelays (5.00s)
    scheduler_test.go:112: timed out waiting for controlled scheduler event
--- FAIL: TestRunContinuesAfterRecoverableErrors (20.02s)
--- FAIL: TestRunFatalWinsAndJoinsAllWorkers (10.01s)
FAIL github.com/suwa68/signal-watch/internal/runtime 40.047s
```

The four recoverable-error cases and both external-cancellation supervisor cases
each failed waiting for events because the skeleton started no workers. All
validation/canceled-context failures above also remained RED. No syntax/import
failure was used as the behavioral RED evidence. Original RED output is in the
session tool transcript; these excerpts retain original pre-gofmt line numbers.

## GREEN validation

Host has no Go; every Go command used the authorized scoped Compose service.
For logged commands below the actual shell wrapper was:

```sh
set -o pipefail
<command> 2>&1 | tee <log-path>
```

Every command exited 0:

1. Initial focused GREEN:

   `docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./internal/runtime -count=1`

   `ok github.com/suwa68/signal-watch/internal/runtime 0.003s`

   Log: `/tmp/signalwatch-task2-green-runtime.log`.

2. Initial race check:

   `docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test -race ./internal/runtime -count=1`

   `ok github.com/suwa68/signal-watch/internal/runtime 1.010s`

   Log: `/tmp/signalwatch-task2-green-race.log`.

3. Existing Pipeline and processor regressions:

   `docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./internal/application ./internal/monitoring -count=1`

   `ok github.com/suwa68/signal-watch/internal/application 0.007s`

   `ok github.com/suwa68/signal-watch/internal/monitoring 0.005s`

   Log: `/tmp/signalwatch-task2-green-pipeline.log`.

4. After self-review strengthened validation test to use an active context (so
   an accidental start is observable), final repeated race validation:

   `docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test -race ./internal/runtime -count=20`

   `ok github.com/suwa68/signal-watch/internal/runtime 1.034s`

   Log: `/tmp/signalwatch-task2-green-race-final.log`.

5. Complete suite, run once before task commit:

   `docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./... -count=1`

   All 14 tested packages passed; two additional packages have no test files.
   Runtime `0.002s`, application `0.006s`, source/html `0.074s`, state `1.159s`.
   No failing or skipped test was observed.

   Complete output: `/tmp/signalwatch-task2-green-full.log`.

`gofmt -w` ran only on the three owned Go files via the same Compose service.
`git diff --check` exited 0. Container creation messages were routine output;
tests emitted no warnings or races.

## Self-review and remaining concerns

- Barrier tests prove both sources start without a timer release, B completes
  multiple regular runs while A is blocked, A's first delay is a full hour only
  after completion, the next A run requires that delay release, and all pending
  fake waits exit before Run returns. Five-second test guards detect deadlocks;
  no scheduling assertion depends on wall-clock sleeps or a multi-hour wait.
- Recoverable errors preserve their report causes and proceed to the next full
  configured interval, followed by a nil successful report.
- Fatal tests preserve original disk error, fatal sentinel, local send deadline,
  and cancellation; barriers exercise fatal-first and sibling-cancellation-first
  ordering. A returned fatal cannot be hidden by a simultaneous caller cancel.
- The real Pipeline integration preserves v1 item continuation: C's failed mark
  does not prematurely cancel D; no completed fatal result is available while D
  blocks. Releasing D allows its successful mark, then fatal supervision joins
  the sibling. This is intentional delayed discovery, not per-item supervision.
- Production wait owns and stops its timer. Scheduler owns no state/OS-signal/
  notification resource, and the four StateStore methods remain unchanged.
- Shutdown remains cooperative: an unresponsive runner or callback can prevent
  Run from joining. The daemon's bounded shutdown policy belongs to Task 3;
  this task deliberately adds no timeout or resource-closing workaround.
- This is implementation self-review and fixture/channel/race evidence only.
  Parent's independent review, broader phase integration, asynchronous CI, and
  genuine fresh-session Task 3 continuation remain outstanding Parent work.

No known blocking implementation concern remains within Task 2.
