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
| 3 — daemon | Next task | Fresh-context execution rehearsal starts from this reviewed integration boundary |
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
