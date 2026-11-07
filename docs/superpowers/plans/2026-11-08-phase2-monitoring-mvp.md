# SignalWatch Phase 2 — proposed architecture and implementation plan

Status: REVIEW DRAFT. This plan does not authorize Phase 2 implementation,
dependency installation, merge, deployment, or real Telegram sends.
The user's request approves scheduling, durable monitoring state, and Docker monitoring runtime in principle,
and explicitly requires stopping for plan review before implementation.

Repository baseline inspected: `suwa68/signal-watch`, clean `main`,
`092b5e47c77b9714336338920c6442e3994c3959`, 2026-11-08 Asia/Taipei.
Documentation/process update: [issue #29](https://github.com/suwa68/signal-watch/issues/29),
branch `feat/architecture-review-gate`. Phase 2 implementation will use a separate
issue/branch after approval. Resume using the
[startup prompt](../../../doc/phase2-development-prompt.md) and
[architecture review record](../../../doc/reviews/2026-11-08-phase2-architecture-review.md).

## Intent and existing foundations

Deliver one operational Go monitoring process: repeatedly collect configured
HTML sources independently, retain silent baseline/seen state across container
replacement, and deliver new items through the existing Telegram pipeline.
Validate four autonomy goals: Dynamic Orchestration, Non-blocking CI,
Cross-session Continuity, and Autonomous Architecture Review.

Inspected AGENTS.md, README, architecture/language/source documentation,
memory-state handoff, notification orchestration, Telegram, HTML resource
safety, CI and Docker guidance, and the relevant implementation/tests/workflows.
Existing SourceConfig already has IntervalSeconds. StateStore already defines
atomic EstablishBaseline and post-delivery MarkItemSeen. Pipeline.RunOnce has a
pipeline-wide overlap guard, not a source-scoped guard. HTML collection already
has a 15-second deadline, 4 MiB body limit and 1,000-item limit. Query catalog
loading already validates all definitions, IDs, registered types and settings.

GitHub searches found historical #5 (memory state), #8 (pipeline), #13 (tunnel)
and #20 (CI), but no matching Phase 2 implementation issue in the returned results.
Recheck before implementation; create a focused issue if still absent.

## Architecture proposal

One new `cmd/signalwatch` daemon in the modular monolith:

    validated source definitions
      -> one sequential schedule loop + existing Pipeline per source
      -> shared SourceRunner / HTML adapter
      -> existing monitoring Processor and itemkey
      -> shared durable StateStore
      -> shared Telegram notifier bound to one destination

Keep SourceConfig -> SourceAdapter -> MonitorItem[] and StateStore's existing
four methods unchanged. Each source gets its own Pipeline so one slow source
does not hold another source's overlap guard. No database transaction or runtime
global lock is held during collection or notification network I/O. Sources are
independent; there is no aggregation or cross-source notification order promise.

### Storage approaches considered

1. Standard-library atomic JSON snapshots: no new module, but requires custom
   rollback/crash consistency, process locking and full-state rewrites per mark.
2. bbolt: focused embedded transactional dependency and built-in file locking;
   viable alternative, but backend key constraints require storage encoding care.
3. SQLite: recommended. Two small tables map existing text source/item keys
   directly, transactions cover baseline and seen writes, and existing tooling
   can inspect the state. Costs a driver dependency and a small ownership lock.

Propose database/sql with a pinned Go-1.25-compatible modernc.org/sqlite release;
it is CGo-free. Review the resulting module graph and confirm Linux arm64/amd64
builds before adopting the pin. No ORM, external DB service or backend selector.
Official references: [SQLite atomic commit](https://www.sqlite.org/atomiccommit.html)
and [modernc SQLite driver](https://pkg.go.dev/modernc.org/sqlite).
Dependency choice remains a proposal.

Private SQLite representation: `initialized_sources(source_id PRIMARY KEY)` and
`seen_items(source_id, item_key, PRIMARY KEY(source_id,item_key))`, plus SQLite's
internal schema-version marker. Atomic baseline inserts keys and initialization
in one transaction, including the empty baseline; an existing initialized source
returns false without inserting the supplied keys. Seen writes are idempotent.
Preserve memory.go's ability to mark a source/item before initialization; do not
add a foreign-key precondition that changes the existing StateStore contract.
Use one database/sql pool per process with SetMaxOpenConns(1) and
SetMaxIdleConns(1), configured before the first query. All calls go through
database/sql, never through a raw driver connection shared across goroutines.
Each write is a short context-aware transaction. Inside a transaction, use only
that Tx's methods, never the parent DB: acquiring a second connection from this
single-connection pool would deadlock. Close rows before any subsequent operation
that needs the connection. Baseline's initialized check, inserts and marker all
belong to the same transaction; validate inputs before mutation just as memory.go
does. No collection, sending, callbacks or long-lived read transaction inside it.

Use rollback journaling (journal_mode=DELETE), synchronous=FULL and a fixed
1,000 ms busy_timeout. Apply connection-local settings to every connection created
or replaced by the driver, using its documented initialization mechanism; verify
the selected pin's settings in tests. Do not add WAL, shared-cache mode, a write
queue, application transaction-retry loop or user tuning switches. Preserve the
whole state directory including journal files.
Initialize only a new/empty DB; reject incompatible schemas and detected
corruption without resetting. No general migration framework.

Driver-selection acceptance must cover transaction cleanup, not just successful
commits. COMMIT returning SQLITE_BUSY may leave a native SQLite transaction active,
while database/sql considers its Tx finished: deferred Tx.Rollback alone is not
proof of rollback. Before releasing a connection for another call, the selected
driver/store path must demonstrably clean up failed transactions or discard the
unusable connection. On failed cleanup, retain both causes, classify ErrFatal and
fail the runtime rather than return the connection with uncommitted keys visible.
Do not add commit retries or a generic transaction framework. Test a real
COMMIT-BUSY induced by a separate read transaction: after cleanup/releasing the
reader, the next store operation must work, see no uncommitted keys, and agree
with a close/reopen check. Fault injection alone cannot close this driver-pin
requirement. See [SQLite transaction failures](https://www.sqlite.org/lang_transaction.html).

The pool serializes brief SQL access, not source collection or delivery. Test
concurrent source initialization/marks and same-source baseline contention, one
baseline winner, no lost seen keys, and canceled waiters. Explicitly test the
single-connection transaction path with a deadline to detect pool reentry deadlock.
See [Go connection management](https://go.dev/doc/database/manage-connections).

One daemon owns one state directory through a nonblocking OS advisory file lock
held for its lifetime. Acquire it before opening/initializing SQLite or contacting
Telegram; retain the same lock-file descriptor until workers finish and the DB
is closed. Keep the lock file in place after release: unlinking it could let a
new process lock a different inode while an existing owner is active. Reuse
golang.org/x/sys/unix already in the module graph
(make it a direct dependency if imported). Supported monitoring runtime: Linux
containers on a local Docker volume; test ownership on Linux and Darwin as
available. Lock contention fails startup; process death releases ownership.
Test contention from a separate process (including directory aliases), refusal
before notifier/collection side effects, release after normal close/startup failure,
and acquisition after killing an owner. SQLite write locks alone cannot prevent
two processes from sending the same unseen item between HasItem and MarkItemSeen.
Ownership covers cooperating daemons using the same local state directory; it
does not coordinate separate volumes or arbitrary tools bypassing the lock.

### Schedule and lifecycle semantics proposed for approval

- Load and validate every definition once before starting work. Reject an empty
  catalog, duplicate IDs, invalid/overflowing duration conversion, unregistered
  types and malformed settings before collection or notifier initialization.
  Reuse existing repository/decoder/schema/catalog validation. Do not move the
  query feature or introduce a new general config subsystem just for this daemon.
- Run every source immediately after successful startup. After its run completes,
  wait its existing interval_seconds before the next attempt: fixed delay,
  no overlap, queued ticks, catch-up, persisted schedule or extra retry timer.
  A source's collection/delivery failures do not stop other sources.
- Use the existing collection limits. Bound each Telegram send with a 20-second
  child context at assembly. Sender-local timeout remains an item failure, so
  later items still run while the RunOnce caller context is active.
- SIGINT/SIGTERM stops starting new work, cancels active contexts, joins source
  loops, then closes the store and releases ownership. Allow 10 seconds before
  reporting unsuccessful shutdown and exiting nonzero; Compose stop_grace_period
  is 15 seconds. Close storage only after workers finish. Contexts cannot forcibly
  interrupt arbitrary non-cooperative code; do not claim a hard Go cancellation
  guarantee beyond process exit. DB close or ownership release errors are retained
  and cause nonzero exit. The existing telegram.New startup validation has its own
  five-second bound and no caller-context API; recheck startup cancellation after
  it returns, release already acquired resources and never start canceled loops.
- Ordinary collection/mapping/send failures and explicitly recoverable state
  failures are logged and attempted again only through the next regular collection.
  Use the classification and precedence below. Startup/config errors exit nonzero;
  signal shutdown exits 0 only if no fatal failure or shutdown timeout occurred.
- Emit startup, source-run completion/error, fatal state error and shutdown logs
  using the standard library. Do not log bot tokens or raw Telegram request URLs.
  No metrics server, dashboard or new health HTTP server.

### Failure classification and aggregate precedence

Keep the four StateStore methods unchanged. Propose transport/DB-neutral
`state.ErrUnavailable` and `state.ErrFatal` error categories in internal/state;
only the SQLite backend maps driver codes to those categories and preserves the
original cause with wrapping. Application and scheduler do not import SQL types,
parse strings, or classify an error merely because it came from a state method.

| Outcome | Classification and response |
| --- | --- |
| Successful collection/transaction | Existing baseline/seen semantics |
| Invalid item identity, mapping, collection or send error | Source/item failure; keep existing continuation semantics and next regular collection |
| Context cancellation/deadline actually governing the state operation | Return context error; not ErrFatal solely because the operation was canceled |
| SQLITE_BUSY after the bounded busy timeout | ErrUnavailable only after verified safe transaction/connection cleanup; retain unconfirmed items and try only on the next scheduled collection; cleanup failure is ErrFatal |
| SQLITE_LOCKED | ErrFatal: unexpected same-connection/shared-cache conflict under this single-connection design, not an excuse for an unbounded retry |
| SQLITE_FULL, IOERR, CORRUPT, NOTADB, READONLY, CANTOPEN, broken schema/invariants, or unclassified backend failure | ErrFatal; preserve underlying cause, no reset/fallback, terminate runtime nonzero after run error is returned |
| Invalid source/key arguments | Preserve existing validation errors; never reclassify them as disk corruption |

Handle extended SQLite codes by primary code, with a narrow documented mapping;
do not hide a genuine I/O/corruption error just because the context was also canceled.
An INTERRUPT caused by the governing context remains cancellation; an unexplained
backend interruption/failure is not silently accepted. See
[SQLite result codes](https://www.sqlite.org/rescode.html).

When RunOnce returns, supervisor precedence is: (1) inspect the full wrapped/joined
error for ErrFatal and latch it; (2) inspect the actual run/root ctx.Err() for
cancellation, never errors.Is(aggregate, context.Canceled/DeadlineExceeded) alone;
(3) otherwise record a recoverable run error and schedule normally. Fatal state
failure wins over concurrent shutdown; join it into the final nonzero result.
Cancel sibling loops on a fatal result; gather all worker results before final
exit classification so an early canceled worker cannot hide a sibling's fatal
error. Shutdown timeout also exits nonzero. No forced mark with Background context.

Preserve pipeline.go's current behavior: a failed mark does not by itself stop
later items while caller context is active. Consequently fatal-state shutdown is
triggered when that run returns, not immediately at the failed mark. The ten-second
shutdown grace begins when shutdown is triggered, not when that first mark failed.
No short fatal-detection bound is promised. More sends
and duplicates can occur before then; 1,000 items with per-send 20-second budgets
can make this delay long. This compatibility trade-off is explicit for approval;
do not quietly introduce immediate-abort semantics or an out-of-band supervisor.

Required tests: BUSY vs LOCKED/fatal code mapping, including real failed-COMMIT
cleanup/connection reuse and fatal cleanup failure; independent source progress
after recoverable failure; mark failure continues C/D; sender-local canceled or
deadline error while caller remains active; joined fatal + caller cancellation;
fatal after another worker has reported cancellation; baseline/read/mark fatal
causes retained through wrapping; sender deadline plus a different item's fatal
mark in one aggregate; cleanup failure with nonzero exit; and pure signal shutdown
with no fatal error. A barrier test holds D after C's mark failure, demonstrates
that the supervisor has not yet received the completed run's failure, then releases
D and observes fatal shutdown. Fault-injection tests prove classification; actual SQLite
transaction/reopen tests prove persistence. Do not conflate these evidence tiers.

### Restart and notification failure semantics proposed for approval

- First successful collection is a silent atomic baseline, including zero items.
  Failed collection/invalid item keys/failed baseline transaction never complete
  a partial baseline. Existing initialized sources stay initialized after restart.
- Reuse the provisional v1 key builder exactly. Persist source initialization and
  seen keys only; no content snapshots, new identity policy or deletion tracking.
- After notification success, commit seen state durably before claiming item
  completion. Failed/uncertain send or successful send followed by failed/uncertain
  mark leaves delivery unconfirmed; a duplicate on later recollection is acceptable.
- A failed item can be retried after restart if it is collected again with the
  same key. If it disappears, there is no saved payload to send. This is not an
  outbox, exactly-once or durable at-least-once delivery guarantee.
- New items first observed after restart are notified for initialized sources.
  Downtime events no longer visible in the source cannot be recovered. New source
  IDs baseline independently. The same source ID retains its prior state across
  definition edits/removal/readdition; no automatic rebaseline or definition hash.
- Bind one destination to the daemon. State remains source/item scoped. A changed
  destination does not replay seen items; operators must keep logical source IDs
  and the intended destination stable. Destination-specific replay/routing is deferred.
- A missing DB in a new state volume is a fresh baseline. Deleting/replacing the
  volume therefore loses continuity; invalid/unwritable existing state must never
  silently fall back to memory or reset.

### Minimal runtime inputs and packaging

Reuse source interval_seconds, `--sources-dir`, existing Telegram token/chat-ID
environment variables and the existing source-directory mount convention.
Add only `--state-dir` (default `/state`) for the necessary persistence location;
derive the DB and ownership-lock paths inside it. Bind a fixed internal destination
ID, not a new routing setting. Timeouts and grace periods above are fixed defaults.
No new source schema, cron expression, worker-count, backend, retry or tuning knobs.

Add Dockerfile.monitor and compose.monitor.yaml with one monitor service: compiled
Go binary, CA certificates, non-root user, read-only source mount/root filesystem,
an owned writable named state volume, init, and restart policy. Verify fresh-volume
permissions explicitly. No host ports, source-code mount or tunnel dependency.
Supply secrets only explicitly at runtime; keep test services credential-free.
Allowlist only required build inputs in .dockerignore. No deployment in this work.

### Docker capability and acceptance feasibility

Before builds/acceptance, probe `docker compose version` and
`docker info --format '{{.ServerVersion}} {{.OSType}} {{.Architecture}}'`.
Separate sandbox denial, approval-review rejection, engine unavailability, build
dependency failure and test assertion failure. Use the normal supported permission
escalation for authorized Docker commands when necessary; never change socket modes,
grant broad host access or mount the Docker socket into the monitor to bypass a
denial. If automatic review rejects the action and no permitted route remains,
record the exact action/reason and report acceptance blocked/unverified.

Observed on 2026-11-08: Compose v5.3.1; sandboxed docker info exited 1 with socket
permission denied; the explicitly scoped read-only probe outside the sandbox
exited 0 and reported Docker 29.5.3, linux/aarch64. This proves engine reachability
for that probe only: builds, runtime creation, volume permissions, architecture
builds and monitoring acceptance remain unexecuted. No current monitor exists.

Run acceptance under an isolated Compose project/volume namespace with
`--env-file /dev/null`, no host proxy variables, no production catalog or secrets,
and no external runtime egress. Host Compose controls containers; monitor gets
only sources, state volume and the acceptance-only CA mount. Pre-build/pull can
use network for public dependencies; keep that evidence separate from offline
runtime assertions. CI uses its Ubuntu Docker engine; local denial cannot be
reported as either a product test failure or a successful test.

## Implementation sequence and review boundaries

After plan approval: recheck/reuse or create the GitHub issue with problem, scope,
non-goals and acceptance criteria. Refresh the approved main base and use an
isolated `feat/phase2-monitoring-mvp` branch (AGENTS.md naming overrides the default).
Save the approved design and detailed checkbox plan under docs/superpowers/;
maintain a progress ledger tied to that plan and commit ranges. No main commits.

1. Durable state deliverable.
   Add internal/state/sqlite.go, sqlite_test.go, ownership files/tests as needed,
   and the driver pin; preserve store.go's interface. Proposed lifecycle API:
   `OpenSQLite(ctx context.Context, directory string) (*SQLiteStateStore,error)`
   and `(*SQLiteStateStore).Close() error`. Add a recognizable backend-failure
   classification for the daemon without putting DB details in core models.
   RED -> GREEN: contract, atomic rollback, empty baseline, no mutation when
   already initialized, idempotence/source isolation, canceled operations, long
   valid IDs/keys, parallel access, reopen, second-owner refusal, process-death
   release and unsupported/corrupt/unwritable state. Commit and review this slice.

2. Scheduled runtime deliverable.
   Add internal/runtime/scheduler.go and scheduler_test.go, assembling source-owned
   pipelines through `RunOnce(context.Context,config.SourceConfig) error` and a
   timer seam. The scheduler accepts configs plus per-source runners and returns
   only when canceled or fatally failed. Keep domain collection outside timers.
   RED -> GREEN: immediate runs, distinct intervals, fixed delay, no overlap or
   catch-up, blocked source A while B progresses, recovery after collection/send
   errors, caller-vs-sender cancellation, duration overflow and shutdown cleanup.
   Tests drive a fake clock and channel barriers, not wall-clock sleeps. Commit
   and review this slice independently of the SQLite implementation where possible.

3. Runnable daemon integration.
   Add cmd/signalwatch/main.go and main_test.go; keep parsing/environment handling
   and signal lifecycle outside the reusable scheduler. Validate catalog, acquire
   state ownership and initialize one real notifier before scheduling. Build one
   pipeline per source with the shared store and notifier. Add assembly tests using
   fakes and local HTML/Telegram HTTP fixtures, including getMe, real renderer and
   sendMessage wire behavior. Keep test transport seams in infrastructure, with
   no production Telegram endpoint flag. Verify logs redact secrets and fatal
   storage failures reach shutdown. Commit and review integrated behavior.

4. Compose acceptance and operational documentation.
   Add Dockerfile.monitor, compose.monitor.yaml and integration/monitor tests;
   adjust .dockerignore and .github/workflows/ci.yaml. Update README, architecture,
   notification/state status docs, doc/ci-reliability.md and add a focused operational
   guide/handoff. Run the unchanged production monitor image/binary on an internal
   Docker network. A local TLS Telegram stub owns the network alias api.telegram.org;
   its ephemeral matching CA is trusted only through standard SSL_CERT_FILE in a
   test overlay. Synthetic credentials exercise the real getMe/sendMessage SDK
   traffic and renderer. The overlay and fixture/control image are test artifacts;
   no production endpoint flag, fake mode or test-modified monitor binary. Deny
   external egress and do not forward host proxy variables. A Docker-Compose-only
   host shell controller executes fixture control/assert subcommands inside the
   test image, so it needs neither host Go nor a Docker socket in the monitor.
   Restart/replace containers while preserving the named state volume; assert
   externally recorded delivery plus persisted state. Await bounded observable
   fixture/run/commit events rather than sleeping a guessed duration.
   Review the complete branch independently. Open draft PR(s) with Closes #N and
   exact validation evidence. Never merge or deploy.

Each implementation task must have its own concrete failing-test evidence, minimal
implementation, passing targeted tests, commit and applicable independent review.
The detailed execution plan will pin scheduler constructor/test seam signatures
after this architecture is approved, rather than pretending they are already agreed.

## Dependencies and permitted parallel boundaries

State and scheduling share existing contracts but not implementation files; both
must pass before real daemon integration. Packaging consumes the runnable daemon;
container restart acceptance consumes packaging. The final review follows the
integrated commit, and required CI follows the PR head.

Parent chooses execution mode and number of agents from actual task coupling,
risk, available slots and observed cost. No fixed agent count. Recommended starting
posture: execute shared integration edits sequentially; delegate bounded ownership/
failure analysis or independent review when useful. If subagent-driven development
is selected, obey its explicit prohibition on parallel implementation subagents and
its per-task review gates. Read-only independent investigations, test-design review
and documentation audit may overlap with implementation when they do not alter the
same state. Inline execution uses executing-plans and a fresh whole-branch reviewer.
No worker spawns duplicate reviewers. Record why parallel or sequential execution
was selected; do not manufacture parallel work merely to demonstrate delegation.

## Acceptance criteria

- Repeated independent source runs continue until shutdown; a blocked/failing
  source cannot prevent another source from running and notifying.
- A/B first baseline sends nothing; restart/recreate with the same volume and
  A/B/C sends exactly C; a further restart sends nothing for already committed C.
- Empty baseline survives restart; the first subsequent item is delivered.
- Failed first collection or aborted baseline has no partial initialized state.
- Failed C with successful D leaves C unseen and D durably seen; after restart,
  recollection attempts C and suppresses D. Failed mark/uncertain send permits
  duplicate delivery, never a false completion claim.
- Killing a process after committed baseline/seen acknowledgements preserves those
  commits. Killing an uncommitted transaction yields all-or-none baseline. Killing
  after send acknowledgement before mark permits the documented duplicate.
- SIGTERM interrupts pending timers and active cooperative network work; no new
  items start after cancellation is observed; workers finish before the DB closes.
  Test normal exit 0 and a noncooperative test double's timeout/nonzero path.
- Invalid catalog, empty catalog, state corruption/incompatible schema/permission
  failure or a second owner fails clearly without live sends or silent state reset.
- Fresh non-root state volume is writable; container recreation retains it; query
  CLI/MCP remain read-only and coexist without monitoring-state side effects.
- Existing Telegram mapping/rendering/order and sender-local cancellation regressions
  remain green. No live websites, real tokens or real Telegram sends in acceptance.
- Focused, full, race, vet and monitoring-container acceptance pass for the final
  reviewed commit, with any unavailable checks reported as unverified.

### Acceptance evidence tiers

| Claim | Artifact and deterministic observation | What it does not prove |
| --- | --- | --- |
| Real scheduling/SDK, baseline, restart/recreation and committed-state retention | Unchanged production image in isolated Compose; wait for successful source-run completion after fixture transitions, then restart or kill; query state safely after stopping and assert the stub's ordered sends | Merely seeing an HTTP request does not prove send returned or mark committed |
| Interrupted baseline is all-or-none | SQLite store integration test with actual rollback injection; a test-built subprocess pauses the actual store transaction before commit through an infrastructure-only synchronization seam, signals a pipe barrier, is killed, then reopened state is inspected | This is a deterministic helper test, not a production-container internal-window observation |
| Successful send then process death before mark permits duplicate | Pipeline/store subprocess harness: successful fake/local send returns, test infrastructure blocks the seen operation before mutation and signals barrier; kill/reopen/recollect and assert duplicate eligibility | No claim that a stub's request timestamp proves this precise window in the unchanged production binary |
| Uncertain remote send may duplicate | Production image plus TLS stub records acceptance but withholds the response; terminate/restart and recollect, asserting unconfirmed completion | Remote acceptance does not prove the SDK observed success |
| Normal ownership/crash recovery | Separate production container/helper holds the same directory; second process fails before getMe/collection; kill owner and reopen/acquire | Not a distributed lock or physical-power-loss guarantee |

Keep deterministic synchronization in test harnesses/private infrastructure seams;
no production pause endpoints, environment knobs, public test flags or modified
monitor image. Each result records command, exit status, source commit/image ID,
actual target architecture, barrier/assertion, and test tier. Cross-build success
is separate from actual execution on a target. No internal crash-window claim
may be inferred from an external request count.

## Independent Architecture Review Gate

Apply the [startup prompt's review gate](../../../doc/phase2-development-prompt.md#independent-architecture-review-gate)
before presenting this non-trivial plan for human approval. A fresh-context reviewer
must inspect actual implementation/tests and existing contracts and challenge state
consistency/concurrency, recovery, cancellation/resources, compatibility, acceptance
feasibility and scope/dependencies. Classify actual findings as Blocking, Major,
Minor or Informational with code references, evidence, concrete failure cases and
minimal corrections. No finding quota or fixed agent count. Parent resolves gaps,
obtains independent re-review of material corrections, and discloses remaining
risks/unverified checks and human decisions in the linked review record.

This gate supplements applicable Superpowers reviews. It neither authorizes
implementation nor requires a heavyweight review for a trivial change. The
documentation-only update itself does not lift the human Phase 2 approval gate.

## Validation of the four autonomy goals

Dynamic orchestration: record task/dependency, ownership, mode/agent choice and why,
start/end commits, actual overlaps, reviewer findings, resolutions and coordination
rework in the ledger. Evidence is the dispatch/review record and conflict-free
integration, not an invented minimum agent count or claimed efficiency percentage.
Verify the applicable review/concurrency gates were met and completed work was not
redispatched after a handoff.

Non-blocking CI: push/open PR asynchronously and record head SHA and run URLs.
Never run gh pr checks --watch or gh run watch in the Parent. While pending,
complete independent review, operational documentation, evidence collation or the
handoff. Use bounded status snapshots with backoff. Record at least one useful
work item completed during an actually pending run; if CI finishes too quickly,
report that overlap as not exercised rather than simulating it. Re-read PR head
and expected checks before final reporting; require success for that exact SHA,
including Test and vet and the new Monitoring acceptance job. Check branch
protection when readable; distinguish project completion checks from GitHub-enforced
required checks. A new push invalidates previous-head evidence. Do not change
branch protection. If CI is still pending, report pending, not complete.

Cross-session continuity: at the first reviewed durable-state + scheduler integration
boundary (before/at daemon integration), commit a concise doc/handoffs/phase2-monitoring-mvp.md
with approved semantics, base/branch/HEAD, issue and PR URLs, task/commit/review map,
actual test commands/results, pending CI head/run/check names, unresolved problems,
operator assumptions and exact next commands. All essential information must live
in tracked artifacts/PRs, not only ignored skill workspace files. Have a fresh-context
executor complete the next bounded implementation task using only the tracked
checkpoint artifacts, repository instructions and code; do not give it the prior
conversation or fill gaps from Parent memory. Review its actual change, validation,
questions and any repeated work against the checkpoint. Fix missing handoff context
and record the outcome. This is a fresh-context resumption rehearsal; actual
cross-session execution is verified only if a genuinely new Codex session performs
the continuation. If only a read-only handoff review occurs, report execution
resumption as unverified. Refresh the handoff at the final integration boundary.

Autonomous architecture review: preserve reviewed code/plan revision, exact
contracts inspected, severity/scenario/evidence, Parent dispositions, material
re-review, residual risks and human decisions. Use the three regression cases:
SQLite concurrency/ownership, recoverable versus fatal state errors (including
mixed cancellation), and Docker permission/acceptance evidence. A review may find
no defect; actual inspection is mandatory, invented findings are not. Demonstrate
that Parent corrects consequential gaps before asking humans and rejects needless
frameworks/configuration. Track later architecture rework attributable to assumptions
the gate missed; the present pre-implementation review cannot yet prove reduced
post-implementation rework or a percentage efficiency gain.

Product evidence: run current CI commands via the development container with an
empty Compose env file:

    docker compose --env-file /dev/null run --rm -T go go test -count=1 -timeout=5m ./...
    docker compose --env-file /dev/null run --rm -T -e CGO_ENABLED=1 -e CC=gcc go go test -race -count=1 -timeout=5m ./...
    docker compose --env-file /dev/null run --rm -T go go vet ./...

Add required bounded monitoring acceptance on Ubuntu; pre-pull/build dependencies,
then deny external runtime network and use event-driven observations, local fixtures
and isolated test project/volume names. Build/runtime image retrieval is separate
from offline acceptance. Test ordinary restart, container replacement and SIGKILL;
do not present these as proof against arbitrary physical disk/power failure.

## Review focus and current evidence limits

Prioritize concurrent same-source ownership, source independence under network
failure, cancellation between send and durable mark, baseline transaction interruption,
and state-volume identity/permissions. Each is covered above by an explicit test.
Also inspect the driver graph/Go compatibility and token leakage in SDK error logs.

Everything above is proposed unless it describes inspected existing behavior.
No product code/tests/builds/CI runs have been performed for Phase 2. The read-only
Docker capability results above distinguish sandbox denial from an approved
successful engine query; runtime creation and monitoring acceptance remain
unverified. Issue #29 and feat/architecture-review-gate concern documentation and
process only, not Phase 2 implementation. Planning does not silently expand approval to
durable retry, outbox, routing, new adapters, new identity/content-diff semantics,
NLP, AI aggregation, dashboard, message brokers or distributed workers.

Human decisions still required: approve SQLite plus its driver/ownership trade-off,
fixed-delay scheduling and timing defaults, restart/recollection delivery limits,
and deferred fatal detection that preserves current Pipeline item continuation.
Approve the final reviewed Phase 2 plan before implementation. Do not mistake the
reviewer's design-gap closure or approval of the documentation PR for that approval.

Review history: the initial builder check clarified production-image transport
and fresh-context execution, but did not require a code-grounded architecture
audit. The subsequent independent architecture review found a Major aggregate
failure-precedence gap and two Minor timing/evidence-tier gaps. Parent corrections
and material re-review are tracked in the linked review record. Design-gap closure
is separate from future implementation tests and acceptance.
