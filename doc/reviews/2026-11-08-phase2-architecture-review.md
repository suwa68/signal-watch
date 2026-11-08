# Independent code-grounded Phase 2 architecture review

Date: 2026-11-08 (Asia/Taipei)
Repository: `/Users/chengewei/horse/signal-watch`
Verified HEAD: `092b5e47c77b9714336338920c6442e3994c3959`; `git status --short` returned no changes.
Proposal reviewed initially: the existing temporary Phase 2 draft, before Parent
corrections. The maintained [revised plan](../../docs/superpowers/plans/2026-11-08-phase2-monitoring-mvp.md)
and [startup prompt](../phase2-development-prompt.md) are now tracked in issue #29's
documentation branch. Original line numbers below refer to that initial draft;
repository code references refer to the baseline commit above. The original
temporary path is not required to resume from the revised artifacts.

## Outcome

No Blocking finding. One Major clarification should be incorporated before human plan approval, with two Minor clarifications. The proposed architecture is feasible and proportionate: per-source sequential loops and Pipeline instances, one shared durable store, one notifier, no new service boundary. This review approves neither implementation nor expanded scope. Correcting this document's proposal findings closes planning gaps only; all Phase 2 execution evidence remains unverified.

The review read implementation and test bodies, not just design documents. No builds, tests, live sources, Telegram calls, Docker containers, GitHub mutations, branch changes, or repository writes occurred. Only this report was written outside the repository. `command -v docker` returned `/Users/chengewei/.rd/bin/docker`; `command -v go` returned no path. Docker engine access was not attempted in this review; the proposal records an earlier sandbox denial, which I have not independently reproduced.

## Findings

### AR-1 — Major: specify fatal-error precedence over joined cancellation and cleanup

**Repository evidence:** `internal/application/pipeline.go:94-99` joins caller cancellation onto any existing error; `:120-128` joins send and mark errors, including several independent causes. `internal/monitoring/processor.go:64-77` wraps baseline/read errors with `%w`. `internal/application/pipeline_test.go:433-477` requires mark-error identity and later-item continuation, while `:480-511` requires canceled marking to remain a cancellation with the original caller context. Sender-local canceled/deadline errors deliberately remain recoverable (`pipeline_test.go:318-367`).

**Proposal gap:** proposal lines 103-108 distinguish backend faults from cancellation, but do not define precedence when the returned error contains both, or when a signal races with a fatal result from another worker. The scheduler test list (177-179) and daemon test list (188-191) do not explicitly cover mixed errors. This is an ambiguity in a plan, not a claim that nonexistent scheduler code is defective.

**Concrete failure scenario:** C sends successfully, its durable mark returns a typed disk error, then SIGTERM cancels the caller while D is in progress. Pipeline returns a join containing both the disk error and context cancellation. A conventional scheduler/CLI implementation that checks `ctx.Err()` or `errors.Is(err, context.Canceled)` first can exit 0 and suppress the fatal state error. Conversely, classifying every error containing `DeadlineExceeded` as storage-fatal would incorrectly stop all sources on a sender-local timeout. Checking only a final select result can also lose a worker's fatal result when cancellation wins the select.

**Minimal correction:** define a typed/sentinel durable-backend error at the storage boundary and explicitly retain/classify the complete error tree. A fatal marker anywhere in a worker result wins over normal signal cancellation and forces nonzero exit; aggregate worker results after join before deciding exit status. Pure caller cancellation is not marked fatal. Sender-local errors and input validation errors are not backend faults. Avoid classifying by error-message text or blanket cancellation checks. Define close/unlock failure reporting: cleanup errors must not turn an unsuccessful shutdown into exit 0. No StateStore interface expansion or core database metadata is needed.

**Evidence to close:** add plan cases for (a) baseline/read/mark fatal errors surviving `%w` and `errors.Join`, (b) sender deadline plus fatal mark in one run, (c) fatal mark plus concurrent SIGTERM, (d) pure signal cancellation => exit 0, (e) sender-local deadline => later item/source continues, (f) cleanup failure => nonzero. Tests should use barriers and fake faults, and verify preserved original causes. Implementation evidence will be required later.

### AR-2 — Minor: make delayed fatal detection under item continuation explicit

**Repository evidence:** `internal/application/pipeline.go:111-128` returns accumulated mark errors only after the remaining items finish; `internal/application/pipeline_test.go:433-469` explicitly protects sending D after C's mark failure. `internal/source/html/extractor.go:16` allows 1,000 items. Proposal lines 94-96 allow 20 seconds per send and 103-107 preserve continuation before supervisor cancellation.

**Concrete scenario:** first item's mark fails, and the next 999 sends each consume their 20-second deadlines. A supervisor that only receives RunOnce's return does not learn the fatal state fault for approximately 19,980 seconds (5 hours 33 minutes), plus local work. Other source loops can continue during this interval. This follows the chosen compatibility policy; it is not an immediate-stop guarantee.

**Minimal correction:** explicitly state that fatal detection happens at completed-run boundaries, that the 10-second shutdown grace starts only once shutdown is triggered, and that no short fatal-detection bound is promised. Keep the existing continuation semantics if intentional. Do not silently change Pipeline's public behavior to solve this. If immediate fatal-stop is later required, it requires an explicit changed daemon policy and separate design, not an unnoticed compatibility change.

**Evidence to close:** a barrier-driven test where mark C fails and D remains blocked should show the intended supervisor timing; releasing D should cause fatal shutdown. Preserve the existing C/D continuation test. No wall-clock multi-hour test is needed.

### AR-3 — Minor: assign deterministic crash-window checks to an evidence tier

**Repository evidence:** `internal/application/pipeline.go:120-124` has adjacent Send and Mark calls, with no external pause or completion hook between them. Existing `pipeline_test.go:480-511` controls this boundary through test fakes. `Dockerfile.tunnel:17-22,36-40` shows a separate integration executable; `integration/tunnel/smoke_test.go:19-25` distinguishes its explicit container execution from ordinary test runs. Proposal lines 197-208 require the unchanged monitor binary and observable events, while acceptance also calls for killing within an uncommitted baseline and between acknowledged send and mark.

**Concrete failure scenario:** a fixture records a Telegram request and the controller kills the production process, then reports that the kill occurred after successful Send but before mark. The HTTP response might not have reached the SDK, or the mark might already have committed. A request observation alone cannot identify that internal window. Similarly, a generic container SIGKILL does not prove that a baseline transaction was open at that instant.

**Minimal correction:** add a small acceptance/evidence matrix. Production-image Compose tests prove real SDK transport, baseline/restart/recreation, committed state retention, permissions, and ordinary SIGKILL recovery. Deterministic uncommitted-transaction and send-success-before-mark windows belong to subprocess/unit integration harnesses with narrowly scoped test synchronization seams, not a modified production image. State exactly which artifact and observable barrier establish each claim. Uncertain-send duplicates can also be tested with the real production binary and a stub withholding the HTTP response, but label that as uncertain send rather than proven post-ack/pre-mark.

**Evidence to close:** list the test tier, trigger/barrier and assertion for each crash case; later record the commands and results separately. Preserve the no-production-test-mode requirement.

## Explicit regression-area assessments

### SQLite concurrent writes and single daemon ownership — Informational; proposal is adequately closed

`state/store.go:10-13` requires one atomic baseline winner without mutation by losers. `state/memory.go:44-73` implements that contract, and `state/memory_test.go:83-114,117-153` exercises losing attempts and 100 concurrent callers. `MemoryStateStore.MarkItemSeen` permits marks independently of initialization (`memory.go:90-107`; `memory_test.go:48-80`), so SQLite must not accidentally add a foreign-key/initialization precondition. Pipeline's mutex is only instance-local (`application/pipeline.go:40-49,90-93`), and HasItem followed by Send is not a claim transaction (`monitoring/processor.go:75-86`; `application/pipeline.go:120-124`).

The proposal's one connection, short transactions, unique source catalog, one Pipeline per source, and lifetime nonblocking ownership lock address ordinary concurrent writes and duplicate daemon delivery without holding transactions over network calls. SourceRunner/adapter sharing is compatible with registry read synchronization (`source/registry.go:58-67`) and per-call HTML state (`source/html/adapter.go:43-84`).

Residual constraint/scenario: advisory ownership protects cooperating daemons using the same state directory, not copied volumes, an operator replacing the lock inode, or independent SQLite writers. Keep the lock file/inode stable for the entire lifetime (do not unlink/recreate it on release); acquire before DB mutation or notifier initialization; release only after store shutdown on normal paths. Keep SQL operations in a transaction on its own connection, never call back through the pooled DB from inside a transaction with a single connection. Treat ordinary marks before initialization consistently with memory.

Minimal action: make these implementation notes explicit if useful; do not add distributed coordination, WAL tuning, retries, or a general storage framework. Later closure evidence: parallel contract/race tests, real subprocess second-owner refusal and process-death release on Linux, reopening persisted baseline/marks, startup error cleanup allowing a subsequent owner. Current proposal already names the main tests. No SQLite concurrency or lock execution evidence exists yet.

### Recoverable errors versus fatal StateStore errors — Major until AR-1 clarified

Existing code preserves typed causes with wrapping/joining, so a storage-specific classification fits without rewriting the Processor or Pipeline. Plain sender failures, sender-local cancellation and mapping failures remain recoverable by contract. Processor errors prevent all deliveries in that collection; mark errors continue later items. AR-1 requires explicit priority and test coverage; AR-2 records the timing consequence of preserving the baseline behavior. No generic retry system is needed.

### Docker sandbox privileges and truthful executable acceptance — Informational; design path exists, runtime unverified

`compose.yaml:3-11` and `doc/ci-reliability.md:24-37` use Docker to provide Go/GCC. `Dockerfile.tunnel:24-33` already provides a non-root runtime precedent, while `compose.tunnel.yaml:29-39` and `doc/ci-reliability.md:49-68` deliberately distinguish offline fixture execution from image retrieval and hosted acceptance. Current required CI is only Test and vet (`.github/workflows/ci.yaml:16-44`), so a future required Monitoring acceptance job must be verified at the actual PR head; existing green CI cannot satisfy it.

The proposed TLS stub at api.telegram.org with an ephemeral CA uses the real private SDK assembly rather than requiring endpoint overrides in production (`notification/telegram/notifier.go:25-41,63-72`). Existing HTTP fixture tests (`notifier_test.go:130-180`) verify getMe/sendMessage wire shape; they do not themselves prove the proposed DNS/TLS/CA image path. The CLI exists locally, but no host Go path was found. The proposal candidly reports the earlier Docker sandbox denial. Container startup/build therefore requires an authorized execution environment with Docker engine access, whether an approved local permission path or the planned Ubuntu CI. Availability of the CLI is insufficient.

Concrete risk: successful local syntax/config inspection or a normal Go test run skips container-only tests and gets reported as monitoring acceptance. Minimal correction/action: retain the explicit unverified status until the dedicated production-image command runs; when local engine access is blocked, record the denial and use authorized CI after implementation approval, without weakening isolation or mounting the Docker socket into the monitor. A final unverified check is an incomplete acceptance condition, not a success. Record image/commit identity, exact command, exit status, actual scenario assertions, and which architecture was run versus merely cross-built. Add fresh-volume UID permissions and a second owner before any getMe/collection as runtime assertions. AR-3 narrows what crash evidence proves.

## Other architecture and lifecycle observations

- No new adapter transport, identity redesign, content snapshots, outbox, routing, NLP, distributed worker, metrics server or configuration framework is required. Scheduling/persistence/Docker remain proposed scope awaiting explicit approval under AGENTS.md.
- Source catalog reuse is appropriate: `query/catalog.go:52-111` validates the catalog, and `config/schema_v1.go:60-70` checks positive interval but not time.Duration multiplication overflow. The daemon-specific overflow/empty checks in the plan are necessary. No need to move query packages just to reuse their loader.
- The shared notifier has no application mutable per-send state (`notification/telegram/notifier.go:20-23,46-74`); the renderer is constructed per call from immutable values. Keep SDK concurrency compatibility in implementation review/race validation rather than introducing a global send lock that destroys source independence.
- Collection already cancels at 15 seconds and closes response bodies (`source/html/adapter.go:44-45,59-71`), while sends accept the caller's context (`notification/telegram/notifier.go:44-51,63-72`). Assembly's per-send timeout must be a child context, cancel promptly after each call, and must not replace the Pipeline caller context for durable marking (`pipeline_test.go:480-511`).
- `telegram.New` has no context argument and documents a five-second getMe timeout (`notifier.go:25-39`). Startup shutdown handling must account for that bounded initialization rather than claiming immediate cancellation. Once it returns, recheck canceled startup context before starting loops and close/release already acquired state. On the 10-second noncooperative shutdown timeout, process exit (not early Close while workers still run) ends ownership; resource cleanup ordering needs a subprocess test.
- Query separation is an existing behavior worth keeping: `query/coexistence_test.go:22-59` proves querying does not consume monitoring items. Reusing catalog validation should not open the state store from query entry points.
- Persisting the existing source/item keys preserves current identity choices (`itemkey/builder.go:18-42`); it does not create a new semantic identity guarantee. The proposal correctly states volume loss, changed destination and disappearing-item limits.

## Inspected materials

Read AGENTS.md and the whole proposed plan. Read implementation/test bodies or relevant sections of:

- `internal/application/pipeline.go`, `pipeline_test.go`, `pipeline_integration_test.go`
- `internal/state/store.go`, `memory.go`, `memory_test.go`
- `internal/monitoring/processor.go`, `processor_test.go`
- `internal/itemkey/builder.go`
- `internal/config/schema_v1.go`
- `internal/query/assembly.go`, `catalog.go`, `coexistence_test.go`
- `internal/source/registry.go`, `html/adapter.go`, `html/fetcher.go`, `html/extractor.go`
- `internal/notification/telegram/notifier.go`, `notifier_test.go`; `cmd/telegram-smoke/main.go`
- `compose.yaml`, `compose.tunnel.yaml`, `Dockerfile.tunnel`, `.dockerignore`, `go.mod`, `.github/workflows/ci.yaml`, `integration/tunnel/smoke_test.go`
- README and relevant sections of `doc/notification-orchestration-v1.md`, `doc/handoffs/memory-state-baseline-v1.md`, `doc/telegram-v1.md`, `doc/ci-reliability.md`, `doc/docker-mcp-tunnel.md`.

No tests were run; reading an existing passing-test expectation is contract evidence only, not a fresh pass result. This report makes no assertion that the SQLite driver, images, sandbox access, crash recovery, CI or autonomy demonstrations have been executed.

## Parent corrections after the independent review

The independent report above is retained as the initial review. This section
records Parent decisions separately; a Parent correction is not independent
verification and does not prove future runtime acceptance.

| Finding | Parent disposition and correction | Required closure evidence |
| --- | --- | --- |
| AR-1 Major — aggregate failure precedence | Corrected plan: backend-neutral ErrFatal/ErrUnavailable; fatal wins over joined cancellation, all worker results retained, cleanup failures nonzero; explicit context/source/driver-code classification | Independent re-review of the revised rules; later wrapped/joined errors, concurrent signal/worker and cleanup regression tests |
| AR-2 Minor — delayed detection | Corrected plan: supervisor learns fatal state failure only when RunOnce returns; ten-second grace starts after shutdown triggers; long batch-drain latency disclosed for human decision | Independent re-review of compatibility; later C/D barrier test, no hours-long sleep |
| AR-3 Minor — deterministic crash windows | Corrected plan: evidence-tier matrix assigns production-image observations separately from synchronized transaction/send-before-mark helper tests | Independent re-review of feasibility; later barrier/commit/image/architecture-specific test results |
| AR-4 Minor — failed COMMIT cleanup (found during re-review) | Corrected plan: BUSY is recoverable only after safe native transaction/connection cleanup; deferred database/sql Tx.Rollback is not assumed sufficient; cleanup failure retains causes and is fatal, with unusable connection discarded | Scoped independent verification; later real external-reader COMMIT-BUSY, next-operation/no-uncommitted-keys/reopen and cleanup-failure checks for the selected driver pin |

Additional Parent clarifications address the user's three regression cases without
adding runtime features:

- **Concurrency/ownership:** SetMaxOpenConns(1)/SetMaxIdleConns(1) before use;
  connection initialization applies settings on replacement; Tx-only SQL inside
  transactions; close rows; no network I/O in transactions. Stable advisory lock
  inode acquired before DB/notifier, held through DB close, process/alias/death
  tests. Preserve MarkItemSeen before initialization; no new foreign-key precondition.
- **Recoverability:** SQLITE_BUSY becomes ErrUnavailable after a fixed bounded
  busy timeout, retried only on a regular collection; SQLITE_LOCKED is unexpected
  under this design and fatal. Storage capacity/I/O/corruption/readonly/schema and
  unknown backend errors remain fatal. Context and validation errors are not
  automatically fatal; simultaneous real backend errors are not discarded.
- **Docker evidence:** reproduced sandbox socket denial (exit 1), then ran the same
  scoped read-only engine probe through the normal approved escalation (exit 0,
  Docker 29.5.3 linux/aarch64). This independently updates Parent environment
  evidence beyond the initial review. It does not validate permission for a future
  build/run, non-root volume behavior, TLS topology or any monitoring scenario.
  The plan states the authorized permission path and keeps all acceptance unverified.

Evidence underpinning the corrections includes
[Pipeline cancellation joining](../../internal/application/pipeline.go#L94),
[send/mark ordering](../../internal/application/pipeline.go#L120),
[atomic baseline contract](../../internal/state/store.go#L10),
[memory baseline](../../internal/state/memory.go#L44), and the actual test bodies
`TestMarkFailureAfterSendAllowsLaterItemsAndRetry`,
`TestCancellationAfterSuccessfulSendDoesNotForceMark`, and
`TestSenderLocalFailureContinuesWhileCallerActive` in
[pipeline_test.go](../../internal/application/pipeline_test.go).
The [Go connection-pool guidance](https://go.dev/doc/database/manage-connections)
and [SQLite code definitions](https://www.sqlite.org/rescode.html) inform proposed
backend behavior; no SQLite backend currently exists in this repository.

## Re-review status

Independent material re-review completed using a fresh-context reviewer. It read
the exact four-file staged diff, inspected actual implementation/test bodies and
primary Go/SQLite/driver sources, and confirmed scope/approval separation. The
initial reviewed plan SHA-256 was
`29b1f27827100d7c58ddf21cd05054301148cb843bd19d77514b732c7eb53eac`.
AR-1, AR-2 and AR-3 were independently closed at design level.

The reviewer then found AR-4, a Minor failed-COMMIT cleanup validation gap. Parent
made the narrow correction in the plan and disposition table. The same independent
reviewer checked that exact delta against the working tree and verified plan
SHA-256 `6a8bba08f9e505542a9b6daee5e88d4610f0d112976eadd034e33e1d51f0c6b6`.
AR-4 was independently closed at design level. The final recorded verdict was:

> No Blocking, Major or Minor design finding remains open.

No architecture content in the plan changed after that verified hash. This status
section records the returned review outcome; it is not a new design correction.
The reviewer did not execute product tests/builds/containers or independently
repeat Parent's Docker probes. Driver-pin validation and every future runtime
acceptance condition remain unverified. No Phase 2 implementation is authorized.

## Remaining risks, trade-offs and unverified acceptance

- SQLite driver version/graph, Go 1.25 compatibility, target builds, real SQL
  cancellation/rollback, failed-COMMIT cleanup and locking remain unverified until
  implementation checks. SQLite's [transaction documentation](https://www.sqlite.org/lang_transaction.html)
  states that COMMIT-BUSY can leave the transaction active; Go's
  [database/sql implementation](https://go.dev/src/database/sql/sql.go) marks the
  Tx finished before the driver Commit result. Selected-driver cleanup is a
  dependency acceptance condition, not a current repository defect or proven pass.
- Retaining the existing Pipeline contract delays fatal-state detection until the
  run returns and can permit additional sends/duplicates. There is no outbox or
  delivery guarantee for disappearing items. This is a human-visible choice.
- Advisory locking covers cooperating daemons on one local directory, not copied
  volumes, noncooperating writers, or distributed/network filesystems.
- Engine reachability is not container acceptance. No monitor image, TLS fixture,
  restart/crash/volume test or required Monitoring acceptance CI check has run.
- Review records demonstrate pre-implementation defect discovery and Parent
  correction. They cannot yet demonstrate less post-implementation rework,
  successful fresh-session implementation, or a measured efficiency improvement.

## Human approval boundary

This paragraph records the review-era boundary; subsequent implementation
authorization is recorded in the execution checkpoint below.

The documentation/process changes are requested. Phase 2 implementation still
requires explicit approval of the reviewed plan, including SQLite/dependency and
ownership choices, completion-based cadence/timing, restart/delivery limitations,
and batch-drain fatal detection. A review verdict or merging this documentation
PR does not satisfy that separate approval gate. Do not auto-merge or deploy.

## Authorized execution checkpoint: state-layer material review

The user authorized Phase 2 implementation on 2026-11-08. Task 1 added SQLite
state in `42b577a`, then its independent code review reproduced two Important defects:
normal cancellation could become fatal through ErrTxDone, and native Close could
precede database/sql's automatic rollback. Existing green suites did not cover
those windows. Parent resumed the original implementer rather than bypass review.

Fix `202688a` assigns one synchronous transaction-termination owner using a private
uncanceled lifetime; normal acquisition/SQL still use caller context and commit
checks cancellation. Actual interrupted INSERT testing also exposed native SQLite
auto-rollback. A cleanup-only SAVEPOINT followed by full Rollback makes both native
states safe without data writes, RELEASE/COMMIT or error-string suppression.
Cleanup errors remain fatal with retained causes.

Independent scoped re-review accepted both findings and the material lifetime/
SAVEPOINT choice, with no new fix-diff findings. Actual focused, state race, full
suite and Darwin state execution passed; Linux amd64/arm64 compiled. See the
[implementation handoff](../handoffs/phase2-monitoring-mvp.md) and committed
[lifecycle regressions](../../internal/state/sqlite_lifecycle_test.go). The known
pre-commit cancellation race window and lock-wait versus filesystem-I/O bound
remain disclosed. This closes the state-layer AR-4 implementation concerns, not
the future scheduler/daemon/production-image acceptance requirements.
