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
| 4 — Compose/acceptance | Complete; independently approved after fix round 1 | Original6bce42f + fix8a2fce4; actual corrected offline SDK/TLS acceptance, full tests and affected race/vet pass; platform evidence below |
| Whole-branch fix wave | I1/M1 implemented and validated; awaiting final scoped review | Required uncertain-send image scenario and bounded clean shutdown record; final evidence below |

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

At that earlier state/scheduler checkpoint no daemon/Compose acceptance had run;
the later Task 3/4 sections record subsequent execution. Distinguish
ordinary suite skips, helper crash windows and unchanged production-image evidence.
Local host has no Go; Docker provides Go/GCC. Native worktree shell writes/git/Docker
may require normal scoped escalation. Do not alter socket modes or mount Docker
socket into monitor. The skill workspace helper required approved write access;
do not rerun it in a sandbox permission-failure loop.

## Next steps

Task 4 fix 8a2fce4 independently passed scoped re-review. The whole-branch review
then identified uncertain-send acceptance and graceful-shutdown logging gaps;
the single fix wave below addresses both. Parent now owns one scoped final
re-review and final push/PR/CI coordination. Verify current-head `Test and vet` and
`Monitoring acceptance` asynchronously; every push invalidates earlier-head
results. No merge, deployment, real Telegram, or additional product scope is
approved by this checkpoint. Tasks 1/2/3 are accepted; do not reimplement them.

Storage remains `state.OpenSQLite(ctx,directory)` with concrete `Close()`,
`ErrUnavailable`/`ErrFatal`, the four StateStore methods, modernc v1.59.0 and
`state.db`/`.signalwatch.lock`. Scheduling remains `runtime.Run(ctx,sources,report)`;
report follows the entire RunOnce return. Query composition keeps
`LoadHTMLCatalogAndRunner` and the original wrapper. No accepted core API changed
for packaging. The task sections below retain historical implementation/review
evidence; the latest whole-branch fix section is the current runtime checkpoint.

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

## Task 4 production image and offline acceptance (2026-11-09)

Implemented from reviewed checkpoint `9285cdada17700535ed5ca5633110297aad9d78b`
on `feat/phase2-monitoring-mvp`, issue #31 / draft PR #32. The Task 4 commit is the
commit introducing this section; use current Git history for its exact SHA.
Validation below ran that implementation tree before its documentation commit,
with the recorded parent SHA plus the working Task 4 diff. Parent independent
task/whole-branch reviews and final-head Actions are still pending. No accepted
state/scheduler/daemon code, tunnel behavior or branch protection was changed.

`Dockerfile.monitor` builds the ordinary binary into scratch with system CAs and
an image-owned 0700 `/state` directory for UID/GID10001. Its dedicated context
allowlist excludes hidden files, tests, catalogs and credentials. Production
Compose has read-only sources/root, capabilities dropped, no-new-privileges,
init, `unless-stopped`, a retained named volume and a15-second stop grace.
[Operations/acceptance](../monitor-compose.md) describes upgrades, state deletion,
restart-loop diagnostics, filesystem assumptions and delivery limits.

One test fixture image provides HTML, ephemeral TLS for `api.telegram.org`,
loopback control and a stopped-volume inspector. The unchanged monitor image
uses its normal SDK with only test-overlay `SSL_CERT_FILE` trust injection.
Resolved configuration asserts internal-only networking, no ports/socket mount,
explicit empty upper/lower proxy variables, literal synthetic credentials,
read-only sources/CA, unchanged command/entrypoint and controlled restart policy.
The production policy is resolved and asserted separately. Fresh-volume ownership
comes from the production image, never fixture chown. Inspector opens `mode=ro`
after a nonblocking ownership lock, checks exact source/key sets and10001:10001
metadata, and is demonstrably refused while the monitor owns the directory.

Executable scenarios proved silent A/B and empty baselines; first post-empty E
sends; retained C suppression after recreation and ordinary restart; beta C
failure with D successful/seen, then only C sent on recollection; beta progress
while alpha's HTTP request remains held; second-owner fatal rejection before
getMe; canceled HTTP requests and clean signal exit0; and ordinary committed
SIGKILL exit137/reopen with exact state and no duplicate sends. The fixture
checks actual SDK multipart fields and exact escaped HTML including the real
renderer, parse mode and disabled previews. Held successful replies establish
this barrier: receive request, capture completed-run cursor, release reply,
observe a later successful RunOnce completion, stop, then inspect SQLite.
No receipt-only inference or wall-clock sleep establishes a commit. Log reads
are bounded snapshots rather than unmanaged follow processes. The second-owner
process has a12-second bound and is reaped. Cleanup is scoped to the generated
project and succeeded; it never prunes images or unrelated resources.

RED evidence: four meaningful fixture tests failed against not-implemented
handlers/barriers/log parsing; the pre-packaging executable acceptance failed
because `compose.monitor.yaml` was absent. Additional resolved-config regressions
failed for missing Telegram TLS alias and an overridden production entrypoint,
then passed after strict validation. Self-review added a failing regression for
a canceled-and-replaced alpha request falsely satisfying a simple held snapshot;
the final check rejects any new held/canceled alpha event during beta progress.
The final full/race/vet/Compose checks were refreshed after that fixture fix.
The first built integration run exposed an
inspector-profile omission in the harness's resolved config; it failed closed,
was corrected, and subsequent complete runs passed. A fixture listener type
compile error was corrected before focused GREEN. These were Task 4 harness
rework, with no application/core corrections.

Final local commands (all exit0):

```sh
integration/monitor/acceptance.sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./... -count=1 -timeout=5m
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T -e CGO_ENABLED=1 -e CC=gcc go go test -race ./... -count=1 -timeout=5m
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go vet ./...
```

Go1.25.14 Linux/arm64, Docker Composev5.3.1. Full suite: daemon10.256s,
fixture0.013s, state1.260s; full race: daemon14.397s, fixture1.044s,
state5.550s; vet no diagnostics. Ordinary tunnel integration remains skipped
without its separate runtime; no new tunnel acceptance is claimed.
Final offline project: `sw-acceptance-1794156958-5967`, successfully cleaned.
Actual monitor image/platform:
`sha256:41738293a6b51d77aebf16398146953d9232f8fe2ef5acadde736e3ac9c53e72`,
Linux/arm64; fixture image:
`sha256:cffaa43fa0656b3e26979d43fee95b9b7949f8aeae9b81bddd4b62dac6690bf7`.
The monitor image ID was identical at scenario start/end. Supplemental ignored
evidence is in `.superpowers/sdd/monitor-acceptance/sw-acceptance-1794156958-5967/`
(resolved models, image/platform IDs, per-stage logs, ledger, exit/cleanup data)
and `.superpowers/sdd/2026-11-08-phase2-execution/task-4-logs/` (RED/GREEN,
full/race/vet/build/native logs). Essential claims above do not depend on scratch.

Cross-builds used the same Docker Go toolchain with `CGO_ENABLED=0`: `go build
-trimpath ./cmd/signalwatch` for Linux/amd64, Linux/arm64 and Darwin/arm64, plus
`GOOS=darwin GOARCH=arm64 go test -c ./internal/state`. Output binaries stayed in
ignored scratch. The Darwin/arm64 monitor `--help` and full state test executable
actually ran on the Darwin/arm64 host and exited0 (including actual-driver,
ownership, COMMIT-BUSY and helper crash tests). Linux/amd64 has compile evidence
only here; Linux/arm64 has actual Docker suite/production-image execution. The
new required CI job will execute its own runner platform after Parent pushes.

This acceptance is committed-state process-kill continuity, not physical power
loss or a deterministic production post-send/pre-mark crash. Task1/3 helper
crash evidence remains a separate tier. No real Telegram/live website, hosted
ChatGPT connection, deployment, universal filesystem support, exactly-once
promise, or current-head remote CI success is claimed. The new15-minute
`Monitoring acceptance` workflow job keeps read-only/credential-free policy and
uploads seven-day synthetic evidence. `Test and vet` and tunnel behavior remain
unchanged. Parent owns final review, fresh-head asynchronous CI and the separate
four-goal autonomy record; no independent reviewer was spawned by this author.

## Task 4 independent review fix round 1 — exact state and signal evidence

Review of `9285cda..6bce42f` found two Important acceptance false-success paths,
not production defects. I1: appending a seen row could recreate a missing source
entry in the inspector's combined map, hiding loss of initialization. I2: the
shutdown cancellation cursor preceded the second-owner check, so a normal
collection timeout during that check could satisfy the later cancellation wait.
The prior saved signal run did show caller cancellation; the finding concerns
what future runs could falsely accept. Both fixes are confined to the harness.

The read-only inspector now asserts the initialized-source set independently
before comparing seen keys. Pre-initialization seen marks remain valid product
behavior; they simply cannot imply an initialization row. Actual SQLite
regressions reproduce correct A/B seen rows with a missing initialized alpha
row, and cover initialized A/B, valid initialized-empty and missing-empty state.
The first case failed RED against the original query logic, then all passed.

Signal cancellation now has its own fresh monitor after the owner scenario.
While alpha is held, the driver captures a completed-run cursor and fixture-event
cursor, verifies continuous hold, then stops the monitor. It requires exactly one
later canceled event, no replacement held request or released page, and exactly
one later alpha completion with `caller context: context canceled` and without
`context deadline exceeded`. Regressions reject cancellation before the barrier,
post-barrier deadline expiry and timeout/replacement; the weaker error-status
completion check failed RED on both timeout cases before correction.

Fix-round validation (all exit 0), using the same Compose development prefix as
above: `go test ./integration/monitor -count=1 -v` (0.031s), GCC-enabled
`go test -race ./integration/monitor -count=1` (1.100s),
`go test ./... -count=1 -timeout=5m` (daemon 10.264s, fixture 0.063s, state 1.214s),
and `go vet ./integration/monitor` (no diagnostics). `bash -n` and
`git diff --check` also passed. Production code/build inputs are unchanged;
earlier platform/build/native and full-race evidence is not presented as newly
rerun here.

`integration/monitor/acceptance.sh` exited 0 with project
`sw-acceptance-1794204230-48469`, including the corrected stopped-state and fresh
signal assertions and successful scoped cleanup. Actual Go 1.25.14 Linux/arm64
monitor image: `sha256:30612227dcacf4cc10fda3527ab97ce6295bc1d88ba6032a1d853ec06c452d40`;
fixture: `sha256:2d7b522a3b37d8883c16b4b638362415a4c8b1ed75ccf628cba740b94f163253`.
The unchanged production image ID remains stable across each run's scenarios.
This validation used HEAD `6bce42f` plus the working fix diff, subsequently
committed; it is not fresh-head remote CI provenance. Ignored RED/GREEN and final
logs are `task-4-logs/review-i1-*`, `review-i2-*`, and `review-fix1-*` under the
plan-local scratch directory. Runtime evidence is under
`.superpowers/sdd/monitor-acceptance/sw-acceptance-1794204230-48469/`, including
`monitor-signal.log` with actual caller cancellation and the cleanup transcript.
Independent scoped re-review approved I1/I2 with no new findings. Whole-branch
review and final-head CI remain gates. Run42571122272 for8a2fce4 was observed
in progress while Parent completed this integration handoff; no watch was used.
The [four-goal autonomy record](../phase2-autonomy-validation.md) preserves actual
observations, unverified claims and Parent rulings for a fresh session.

## Whole-branch fix wave — uncertain send and graceful shutdown (2026-11-11)

The independent review of `3404598..b9f0c19` found Important I1: the approved
spec's uncertain-remote-send production-image scenario was missing; the earlier
conditional reading of the task brief did not authorize its omission. Minor M1:
a clean signal lacked the specified shutdown log. One author addressed both in
one fix wave without new production test seams/configuration or delegated work.
The usage-limit interruption was resumed from saved working edits and RED/GREEN
logs; it did not start another implementation/review loop. Parent's separate
autonomy document was left untouched.

The new initialized-source U scenario records one valid TLS send while withholding
its success response, asserts it is still held, SIGKILLs the ordinary monitor,
and observes exit 137 plus cancellation. It asserts exactly one first attempt,
no released/success/failure response and no completed alpha run, then inspects
the stopped state and confirms U absent. Same-volume recreation/recollection
attempts U again, releases only the second response, waits for successful run
completion and verifies U persisted. Another restart suppresses U. This is
uncertain remote send/duplicate eligibility, not a precise SDK-success/pre-mark
window; synchronized helper-tier evidence remains separate. Fixture regressions
cover still-held, canceled, released, repeated and stale-cursor cases. Explicit
release events now make unreleased-response evidence auditable.

The sole production change adds one static global
`{"event":"shutdown","status":"success"}` stdout record after `execute` returns
with successful worker join/store cleanup. Error/timeout paths retain fatal
logging without a success shutdown record. Success and fatal final records share
the existing 25 ms best-effort flush, including mutex/output waits; no queue,
configuration or longer shutdown grace was introduced. Blocked output can still
lose the final record. RED tests showed missing records for clean/init/timer
signals; GREEN confirms ordering, no success record on cleanup failure, and
bounded blocked graceful logging alongside existing blocked-operational/fatal
shutdown coverage. Actual signal subprocesses also verify the record.

After resume, Docker's configured socket was absent because the existing Rancher
Desktop runtime was stopped. Starting that app restored Docker 29.5.3; no socket
permissions or runtime configuration were changed. The initial unavailable-engine
invocation is not a test result. Final validation used Go 1.25.14 Linux/arm64 and
Compose v5.3.1. All these commands exited 0:

```sh
integration/monitor/acceptance.sh
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go test ./... -count=1 -timeout=5m
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T -e CGO_ENABLED=1 -e CC=gcc go go test -race ./... -count=1 -timeout=5m
docker compose --env-file /dev/null -p signalwatch-phase2-dev run --rm -T go go vet ./...
```

Full suite: daemon 10.313s, fixture 0.132s, state 1.527s. Full race: daemon
14.351s, fixture 1.126s, state 5.387s. Vet emitted no diagnostics; ordinary tunnel
integration retains its image-gated skip. Production logging changed, so this
fresh full/race/vet run supersedes prior-code evidence. Linux amd64/arm64 and
Darwin arm64 monitor builds also passed. A fresh Darwin arm64 command test binary
ran clean/init/timer, blocked graceful/operational and fatal-shutdown checks
natively, and the native command `--help` exited 0. Linux amd64 remains local
compile-only evidence; Linux arm64 has actual image execution.

Final offline project `sw-acceptance-1794339236-63779` passed and was cleaned.
Monitor image: `sha256:73eed504ad034623edf3b266b98dc9816f5d8d82c75af6e78299d40c6e31260e`;
fixture: `sha256:0c94480500a0a24d4bffcb186cf5719ed9e75b17e5557bd7545a57c8fc07c3e6`.
Image IDs remained stable throughout the scenarios. U's recorded ledger was
send 87, canceled 89, send 96, released 97, replied 98; only the second attempt
had a completed success and persisted key. `monitor-signal.log` contains one
actual global shutdown/success record. Evidence records HEAD `b9f0c19` plus the
working fix diff, subsequently committed; it is not remote final-head CI.
Ignored evidence: `task-4-logs/whole-*` under the plan scratch directory and
`.superpowers/sdd/monitor-acceptance/sw-acceptance-1794339236-63779/`.
The ignored `whole-branch-fix-report.md` lists exact commands and limits.

The required uncertain-send scenario is now present. No live Telegram/site,
physical-power-loss, exactly-once delivery, genuine new-session execution or
current-head Actions success is claimed. Parent owns the single scoped final
re-review and fresh-head CI; no merge or deployment is authorized.
