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
| 2 — scheduler | Not started | Must follow Task 1 independent review |
| 3 — daemon | Not started | Fresh-context execution rehearsal after Tasks 1/2 reviewed |
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

Task 1 produces `state.OpenSQLite(ctx, directory) (*SQLiteStateStore,error)`,
`(*SQLiteStateStore).Close() error`, and `state.ErrUnavailable`/`state.ErrFatal`.
It preserves all four StateStore methods, including marks before initialization.
Private files are `state.db` and `.signalwatch.lock`; never unlink the lock file.
Driver pin: modernc.org/sqlite v1.59.0 (Go1.25), with direct x/sys and driver-required
indirect modules. A private pinned sql.Conn per transaction is discarded after
failure; normal SQL stays on Tx. Independent review found that database/sql's
automatic rollback can race native discard and that sql.ErrTxDone can wrongly
make normal cancellation fatal. Fix202688a uses a private uncanceled transaction
lifetime with original caller context for acquisition/normal SQL and pre-commit
cancellation check. A cleanup-only SAVEPOINT before full Rollback handles native
SQLite auto-rollback after interrupted INSERT; no user-data write or commit begins
after observed cancellation. All cleanup faults remain fatal with causes. These
load-bearing corrections were independently accepted in scoped re-review, with
both Important findings addressed and no new fix-diff findings. Cancellation
after the pre-commit observation can race an already-running synchronous Commit;
the busy timeout bounds lock waiting, not all filesystem I/O. Do not broaden the
state-layer evidence into a whole-daemon shutdown guarantee.

Implementation test report includes actual COMMIT-BUSY + safe reuse/reopen,
concurrent baselines/marks, trigger-induced rollback, separate-process alias/owner
refusal/death release, and killing a real baseline transaction through a test-only
SQLite scalar function/TEMP trigger (no product callback). State race passed;
uncached full repository suite passed. These are Task 1 evidence, not daemon or
production-image acceptance. Verify the independent review disposition before
continuing. Full task logs/report remain in plan-local scratch until results are
captured at the reviewed integration boundary. Final Task 1 commands were:

```sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test -race ./internal/state -count=1 -timeout=90s -v
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./... -count=1 -timeout=120s
```

They passed (state race5.430s; full suite state1.215s). Final focused lifecycle/
COMMIT-BUSY tests passed1.106s. Linux amd64/arm64 + Darwin arm64 test binaries
compiled; Darwin arm64 binary actually executed with all state tests passing.
Tests now committed in sqlite_lifecycle_test.go cover cancellation before/after
statements and during an actual interrupted INSERT, rollback-before-close,
independent fatal causes and cleanup-control failure. Existing actual-driver
COMMIT-BUSY and process ownership/crash tests remain passing.

No daemon/Compose MVP acceptance is claimed at this checkpoint. Distinguish
ordinary suite skips, helper crash windows and unchanged production-image evidence.
Local host has no Go; Docker provides Go/GCC. Native worktree shell writes/git/Docker
may require normal scoped escalation. Do not alter socket modes or mount Docker
socket into monitor. The skill workspace helper required approved write access;
do not rerun it in a sandbox permission-failure loop.

## Next steps

Parent accepts only a committed task plus actual test evidence and independent
spec/quality review; no parallel implementation agents under the selected SDD skill.
Continue the first incomplete task from the execution plan/ledger and existing
commits; never recreate completed work. Refresh this handoff after reviewed Tasks
1/2 with actual APIs/commits/tests and exact Task 3 entrypoint before handing it to
a fresh-context executor. Report a rehearsal separately from genuine new-session
execution. Final acceptance requires current-head Test and vet plus the new
Monitoring acceptance job and a broad independent review; do not merge or deploy.
