# Phase 2 monitoring MVP — execution handoff

## Authorization and location

The user instructed "start phase 2 implementation" on 2026-11-08 after merging
PR #30. Execute the [reviewed architecture](../../docs/superpowers/plans/2026-11-08-phase2-monitoring-mvp.md)
and [execution plan](../../docs/superpowers/plans/2026-11-08-phase2-execution.md),
following AGENTS.md and applicable Superpowers skills. Do not repeat the initial
planning/approval gate. Scope expansions still require the applicable gate.

- Issue: [#31](https://github.com/suwa68/signal-watch/issues/31).
- Draft PR: [#32](https://github.com/suwa68/signal-watch/pull/32).
- Branch: `feat/phase2-monitoring-mvp`; approved base `340459825bc25855663025c56d88f9f046207aa4`.
- Managed worktree: `/Users/chengewei/.codex/worktrees/phase2-monitoring-mvp/signal-watch`.
- Reconcile current `git log`/status and PR head before resuming; do not assume a
  historical SHA below is the latest commit.

## Non-negotiable behavior

One Go process, one Pipeline and fixed-delay loop per source, shared store and
notifier. Reuse interval_seconds. Atomic silent initial baseline (even empty);
retain initialized sources/seen keys across restart; send before durable mark.
Preserve existing item keys and item-continuation/caller-context semantics.
No outbox or guaranteed delivery for disappearing items; uncertain sends or
failed marks can duplicate. Fatal detection occurs after RunOnce returns and
may be delayed by remaining items; this disclosed trade-off is approved.

SQLite uses one connection, short transactions, DELETE/FULL, busy_timeout=1000ms,
and stable lifetime directory ownership before DB/getMe. Safe COMMIT-failure
cleanup is required before connection reuse. Errors must retain causes; fatal
state/cleanup results win over concurrent cancellation. Join workers before DB
close; normal signal shutdown exits 0, fatal/cleanup/10-second grace timeout
nonzero; Compose grace 15 seconds. Send child context is 20 seconds; mark keeps
original caller context. Keep Query CLI/MCP independent of monitoring state.

No real Telegram sends, secrets, new adapters, outbox/retries, identity redesign,
aggregation, NLP, brokers/distributed workers, merge or deployment.

## Current task/review map

| Task | Status | Evidence |
| --- | --- | --- |
| Execution checkpoint | Committed | c13bfab; user approval and concrete task interfaces recorded |
| 1 — durable state | Complete; independently approved after fix round 1 | Original42b577a + fix202688a; final focused/state race/full/Darwin execution and Linux amd64/arm64 builds pass |
| 2 — scheduler | Complete; independently approved | 206c71b + tracking-only9c7e14a; runtime/race/Pipeline/full checks passed; no review findings |
| 3 — daemon | Complete; independently approved after fix round 1 | Fresh executor from2840fe5; initial12bf8d0 + fatal-boundary fixb7515ca; actual command/race/vet and helper evidence below |
| 4 — Compose/acceptance | Not started | Read-only fixture feasibility investigation can overlap earlier tasks |

The Parent's plan-local progress ledger/briefs/reports are under
`.superpowers/sdd/2026-11-08-phase2-execution/`. That directory is ignored scratch;
essential committed progress/results will be mirrored here at integration boundaries.

## Actual validation and pending evidence

Baseline on 3404598: existing uncached full Go suite passed in Docker. An unrelated
tunnel container produced a Compose orphan warning; no existing runtime was
removed. Use dedicated project `signalwatch-phase2-dev` for later development tests.

Early PR CI run [42413107894](https://github.com/suwa68/signal-watch/actions/runs/42413107894)
was observed in progress for c13bfaba7b9a334a8ac501c4a360a2746dd12db2 while
implementation and read-only feasibility work continued. This is evidence of
non-blocking work overlap, not final-head validation. Every push invalidates
previous-head check evidence. Never use Parent watch commands.

Task 1 evidence includes actual-driver COMMIT-BUSY/reopen, concurrent baselines/
marks, failed insert rollback, separate-process ownership/alias/death release and
killing an actual uncommitted baseline. Two cancellation cleanup defects found by
independent review were fixed and independently accepted in202688a; details and
limitations are in the [review record](../reviews/2026-11-08-phase2-architecture-review.md).
These are state/helper-tier results, not daemon/production-image acceptance.

Final Task 1 commands:

```sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test -race ./internal/state -count=1 -timeout=90s -v
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./... -count=1 -timeout=120s
```

They passed (state race5.430s/full state1.215s; focused lifecycle/BUSY1.106s).
Linux amd64/arm64 + Darwin arm64 compiled; Darwin arm64 actually executed all state
tests successfully. Linux amd64 has build evidence, not native execution evidence.

Task 2 commands used the same Compose prefix: `go test ./internal/runtime -count=1`
(0.003s); `go test -race ./internal/runtime -count=20` after strengthened validation
tests (1.034s); `go test ./internal/application ./internal/monitoring -count=1`
(0.007s/0.005s); `go test ./... -count=1` (runtime0.002s/state1.159s). All passed.
Independent review checked code and logs, found no task findings and did not rerun
suites. It confirmed fatal-first aggregates, all-worker join, real Pipeline's
delayed fatal C/D regression, independent fixed delays and timer cleanup.

No daemon/Compose MVP acceptance is claimed at this checkpoint. Distinguish
ordinary suite skips, helper crash windows and unchanged production-image evidence.
Local host has no Go; Docker provides Go/GCC. Native worktree shell writes/git/Docker
may require normal scoped escalation. Do not alter socket modes or mount Docker
socket into monitor. The skill workspace helper required approved write access;
do not rerun it in a sandbox permission-failure loop.

## Next steps

The next executor implements Task 3 only from this tracked handoff, the execution
plan/Task 3 brief, AGENTS.md and actual code. This is a fresh-context rehearsal;
do not claim genuine new-session execution. Record gaps before relying on Parent
conversation memory. Start with `git status --short --branch` and
`git log -6 --oneline`; skip completed Tasks 1/2. Parent commits this handoff before
dispatch; the commit containing this checkpoint is the Task 3 BASE.

Storage APIs: `state.OpenSQLite(ctx,directory) (*SQLiteStateStore,error)`, concrete
`Close() error`, and `state.ErrUnavailable`/`state.ErrFatal`. Keep the four methods
unchanged, pre-init marks allowed, files `state.db`/`.signalwatch.lock`, driver
modernc.org/sqlite v1.59.0. Do not alter accepted store lifetimes for assembly.

Task 2's fixed integration API is `runtime.Run(ctx, []ScheduledSource, report)
error`, with `ScheduledSource{Config config.SourceConfig; Runner Runner}` and
`Runner.RunOnce(context.Context, config.SourceConfig) error`. The report function
receives config and completed run error (including nil), is optional, concurrent
across source workers and must return promptly. Zero source list/blank IDs,
duplicates, nil runners and duration overflow are rejected before goroutines.
It owns source-loop coordination only, not signals, storage or notifier lifetimes.

For Task 3, existing `query.LoadHTMLCatalog` returns a Query Service, not a raw
Pipeline Collector. The execution plan specifies one small shared composition
helper `query.LoadHTMLCatalogAndRunner(ctx,directory,client)` returning catalog and
SourceRunner, with the old loader retained as wrapper. Preserve Query entrypoints,
validation and state isolation; do not copy the registry/validation block or pass
query DTOs through monitoring.

Parent accepts only a committed task plus actual test evidence and independent
spec/quality review; no parallel implementation agents under the selected SDD skill.
Implement cmd/signalwatch's CLI/env validation, one Pipeline per source, shared
store/notifier, safe structured logs and bounded signal lifecycle. Write failing
tests first, preserve query APIs through the helper above, use private test seams
without production fake flags, then run Task 3 checks/commit/report for review.
Task 4's production image/TLS fixtures/Compose and Monitoring acceptance job remain
unimplemented. Final acceptance requires current-head Test and vet plus the new
Monitoring acceptance job and a broad independent review; do not merge or deploy.

## Task 3 fresh-context continuation rehearsal (2026-11-08)

A fresh-context executor actually implemented Task 3 from tracked BASE
`2840fe5230a5fbeb6ac19173abe80adf63715778`, the tracked handoff, execution plan,
AGENTS.md and code. It reported the next task, invariants and validation before
coding. No consequential handoff gap or prior-conversation assumption was needed;
accepted Tasks 1/2 were not reimplemented. This demonstrates a fresh-context
execution rehearsal, **not** a genuinely new Codex session. Independent Parent
spec/quality review is still required before Task 3 is accepted.

New composition is `cmd/signalwatch`: only `--sources-dir` and `--state-dir`
(default `/state`), existing Telegram token/chat environment variables, one
shared SourceRunner/store/notifier and one Pipeline per source. The one added
query API is `LoadHTMLCatalogAndRunner(ctx,directory,client)`; existing
`LoadHTMLCatalog` remains a wrapper with its signature unchanged. Configuration
and intervals validate before state ownership, then getMe. SDK types remain in
the Telegram package. Every send has a canceled-on-return 20-second child;
marks keep the worker context. The accepted store and scheduler were untouched.

Signal supervision covers startup and runtime. Workers join before store close;
pure signal exits0, mixed fatal/cancellation and cleanup errors exit1. Actual
subprocesses tested SIGTERM during initialization (private notifier seam),
in-flight local HTTP requests, timer waits, fatal results, cleanup failures and
the real fixed10-second noncooperative grace. The SDK's documented five-second
getMe initialization bound remains unchanged; actual SDK/TLS transport acceptance
belongs to Task 4.

Operational JSON contains per-source startup and `source_run_completed` events
only after the entire Pipeline returns. Source IDs/errors are bounded, tokens
and Telegram request URLs redacted. RED tests exposed help incorrectly continuing
startup and JSON control-character escaping exceeding the log bound; both were
corrected. Self-review then found a new implementation defect: a blocked stdout
writer held the log mutex, and a synchronous timeout diagnostic waited on it,
defeating shutdown. A barrier regression failed at250ms with a shortened20ms test
grace, then passed after the final diagnostic received a25ms best-effort flush.
This was new self-review rework, not missing handoff context. Normal operational
logging can backpressure workers; final diagnostics may be lost if output is
blocked. The process still exits after its10-second grace plus at most the25ms
final flush, without closing an active worker's store. No queue/framework or
production tuning flag was introduced.

Real SQLite/local HTML/real renderer integration proved silent A/B baseline,
C delivery, cancellation of the send child without contaminating the mark
context, retained suppression after reopen and Query collection not consuming
monitoring state. An actual owned directory prevented notifier initialization.
A synchronized **helper-tier** subprocess used the real Pipeline and SQLite,
paused at `MarkItemSeen` entry after the injected sender returned success but
before SQLite mutation, then was SIGKILLed. Reopen/recollection delivered C
again while baseline A/B stayed suppressed. This is not production-image
post-send observation or a real Telegram delivery.

Final validation used Docker Go1.25.14 on Linux/arm64 (host Go unavailable):

```sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./cmd/signalwatch ./internal/application ./internal/query ./cmd/source-query ./cmd/signalwatch-mcp -count=1 -timeout=90s
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./... -count=1 -timeout=120s
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T -e CGO_ENABLED=1 -e CC=gcc go go test -race ./cmd/signalwatch ./internal/application ./internal/query -count=1 -timeout=120s
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go vet ./...
```

All passed. Final full suite: daemon10.269s/state1.193s; final scoped race:
daemon14.302s/application1.020s/query1.010s; vet emitted no diagnostics. The full
suite and scoped race were repeated after the blocked-log fix; prior results
were not reused for the amended code. `git diff --check` passed. No live sources,
real Telegram sends, secrets, store/scheduler edits, push, merge or deployment.

Next: Parent independently reviews the committed Task 3 diff and actual logs,
records acceptance or requests fixes, then dispatches Task 4 from the tracked
execution plan. Production monitor image/Compose/CI acceptance, final all-package
race/build/platform checks, current-PR-head CI and whole-branch review remain
pending. Ignored Task 3 reports/logs are supplemental; this section preserves
the essential continuation, validation and remaining-work evidence in Git.

## Task 3 independent review fix round 1 — fatal shutdown boundary

Independent review of `2840fe5..12bf8d0` found Important I1: the command grace
observed only OS-signal cancellation. The scheduler's fatal path canceled its
private worker context, then could block in completion logging or joining a
noncooperative sibling; without an OS signal the command never started its
grace. This is implementation review rework, not missing prior-conversation
input. The fresh-context execution remains a rehearsal, not genuine new-session
evidence.

The command now derives a context with cancellation cause from its signal
context. A small command-owned Runner wrapper observes each completed Pipeline
result and cancels that governing context on wrapped/joined `state.ErrFatal`,
**before** scheduler reporting or joining can block. The wrapper returns the
original error unchanged; cooperative scheduler return retains all fatal causes
and exits1. Timeout diagnostics join an available fatal cancellation cause with
the grace error. Pure signal still exits0 and cleanup errors still exit1. The
report callback remains reporting-only, and accepted store/scheduler internals
are unchanged. Store close still occurs only after scheduler workers finish;
timeout does not close an active store. The10-second production grace and25ms
best-effort final flush are unchanged; backpressure/diagnostic-loss limits remain.

RED used real Pipeline + runtime.Run, local HTTP, and a test-only fatal baseline
store seam with **no OS signal**. Both a blocked completion log and a sibling
blocked in its store call failed to exit at250ms using a20ms private test grace;
a cooperative fatal test also proved governing cancellation/cause was absent.
After correction both blocked cases exited1 before release while store Close
remained uncalled; releasing barriers then allowed normal worker join/Close.
Cooperative completion retained the original sentinel plus ErrFatal in the
command cancellation cause and final error. Focused RED command:

```sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go sh -c 'gofmt -w cmd/signalwatch/main_test.go && go test ./cmd/signalwatch -run "TestFatalCompletionWithoutSignal|TestCooperativeFatalCompletion" -count=1 -timeout=15s -v'
```

Exit1 before fix (daemon0.512s), then the same selected tests passed after
formatting main.go/assembly.go (daemon0.073s). Final affected checks:

```sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./cmd/signalwatch -count=1 -timeout=90s -v
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T -e CGO_ENABLED=1 -e CC=gcc go go test -race ./cmd/signalwatch -count=1 -timeout=120s
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go vet ./cmd/signalwatch
```

All exited0: focused10.194s, race14.381s, vet no diagnostics. These runs include
the original signal0/fatal+signal/cleanup precedence, blocked final diagnostic,
actual10-second subprocess timeout, HTTP cancellation, SQLite/query integration
and post-send helper crash coverage. The prior full-suite evidence above applies
to12bf8d0; no new full-suite claim is made for this scoped command-only fix.
Parent scoped independent re-review accepted I1 as addressed, with no new
findings. It verified command cancellation before reporting/join, retained fatal
causes and no active-store Close on timeout; no test reruns were needed. Task 4
is the next incomplete task and remains unimplemented at this checkpoint.
