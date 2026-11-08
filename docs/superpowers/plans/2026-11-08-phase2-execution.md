# Phase 2 Operational Monitoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the approved smallest operational monitoring MVP and evidence for all four autonomy goals.

**Architecture:** One Go daemon owns a SQLite directory and one sequential fixed-delay loop/Pipeline per source. The existing collector, baseline/seen contract and Telegram delivery remain unchanged; cancellation joins workers before store close.

**Tech Stack:** Go 1.25, database/sql with a compatible pinned modernc SQLite driver, existing Telegram SDK, Docker Compose, local HTTP/TLS fixtures.

**Spec:** [Reviewed architecture and semantics](2026-11-08-phase2-monitoring-mvp.md), [architecture review](../../../doc/reviews/2026-11-08-phase2-architecture-review.md).

**Authorization:** User explicitly instructed "start phase 2 implementation" on 2026-11-08 after PR #30 merged. This approves execution of the reviewed proposal, including its disclosed trade-offs. Parent chooses sequential subagent implementation with independent task reviews. Issue #31; branch feat/phase2-monitoring-mvp; base 340459825bc25855663025c56d88f9f046207aa4.

## Global Constraints

- Preserve SourceConfig -> SourceAdapter -> MonitorItem[] and the four StateStore methods.
- Silent atomic initial baseline, including empty; mark only after successful send; keep provisional item identity and existing item-continuation semantics.
- One database/sql pool connection, short transactions, no network calls in transactions; stable lifetime single-directory process ownership.
- journal_mode=DELETE, synchronous=FULL, busy_timeout=1000 ms; safe failed-COMMIT cleanup before reuse; ErrUnavailable only for safely cleaned BUSY; hard/unknown faults and failed cleanup are ErrFatal.
- Fatal errors in any wrapped/joined worker result win over cancellation; join workers before closing storage; normal signal shutdown 0, fatal/cleanup/10-second shutdown timeout nonzero; Compose grace 15 seconds.
- Immediate source runs, then existing interval_seconds after completion; no overlap/catch-up; collection's existing 15-second limit and per-send 20-second child context.
- No live websites, real Telegram sends, committed secrets, outbox/durable retry, new adapters, identity changes, aggregation, routing, NLP, brokers, distributed workers, auto-merge or deployment.
- Add only --state-dir (default /state), reusing --sources-dir and existing Telegram token/chat environment variables; no production test-mode or endpoint flag.
- Independent task and whole-branch reviews, current-head asynchronous CI, tracked checkpoint/fresh-context continuation and truthful evidence tiers.

## Review Focus

- Canceled waiters and failed COMMIT must not leak transactions or expose uncommitted keys; Task 1 actual-driver/reuse tests.
- A second process must fail before DB mutation or getMe; Task 1 process tests and Task 3 startup-order test.
- Joined fatal/cancellation and late worker results must not exit successfully; Task 2 barrier tests and Task 3 exit tests.
- SIGTERM during SDK initialization/noncooperative work must honor bounded lifecycle without premature DB close; Task 3 subprocess tests.
- External HTTP observation does not establish an internal crash window; Task 4 separates unchanged production-image acceptance from synchronized helper tests.

## Task 1: Durable StateStore and directory ownership

**Files:** create internal/state/errors.go, sqlite.go, sqlite_test.go, ownership_unix.go and focused transaction/ownership test files as needed; modify go.mod/go.sum only for the focused driver and direct x/sys import.

**Interfaces:** preserve StateStore. Produce OpenSQLite(ctx context.Context, directory string) (*SQLiteStateStore, error), (*SQLiteStateStore).Close() error, state.ErrUnavailable and state.ErrFatal (wrapped causes retained). No exported SQL/raw-connection types.

- [ ] Write failing contract tests: atomic A/B + initialized persistence; empty baseline; pre-initialization MarkItemSeen; losing baseline inserts nothing; duplicate mark; source isolation; invalid/canceled input; concurrent initialization/marks; no partial baseline after induced insert/commit failure; corrupt/unexpected schema never resets.
- [ ] Write process ownership tests: second owner and alias refusal; lock precedes DB changes; close/startup failure releases ownership; process death releases OS lock; stable lock inode is never unlinked. Use real local temp directories and subprocess barriers.
- [ ] Run focused tests in Docker; record expected undefined/missing implementation RED.
- [ ] Implement minimal SQLite backend and select a Go-1.25-compatible driver pin from actual module/source evidence. Pool max-open/max-idle 1, initialize every replacement connection's PRAGMAs. All transaction SQL stays on Tx; no nested pool acquisition. Keep validation behavior and pre-init marks; reject unsupported/corrupt state without repair.
- [ ] Add actual-driver tests for busy during COMMIT induced by a separate read connection. Prove safe rollback/discard, next operation recovery, no uncommitted visibility and close/reopen agreement; failed cleanup fatal. Verify BUSY/LOCKED/IOERR/FULL/CORRUPT/NOTADB/READONLY/CANTOPEN/unknown and context classification without strings; preserve simultaneous fatal/context causes.
- [ ] Add deterministic helper process test that pauses an actual store baseline transaction before commit, signals a barrier and is killed; reopen and assert all-or-none state. Keep synchronization private/infrastructure only, not a product flag.
- [ ] Run targeted state tests, state race tests and full suite once before committing. Confirm Linux amd64/arm64 builds for the driver. Record exact RED/GREEN output, module graph/pin justification and deviations in task report.
- [ ] Commit this task and report for independent spec/quality review. Do not begin scheduler work.

## Task 2: Independent scheduler and error supervision

**Files:** create internal/runtime/scheduler.go and scheduler_test.go (split focused clock/supervisor tests if useful); consume state/errors.go unchanged.

**Interfaces:** define Runner with RunOnce(context.Context, config.SourceConfig) error; ScheduledSource {Config config.SourceConfig; Runner Runner}; Run(ctx context.Context, sources []ScheduledSource, report func(config.SourceConfig,error)) error. report observes every completed run including nil errors. Use a private injectable wait/timer seam; production API has no tuning/test flags.

- [ ] Write failing fake-clock/channel-barrier tests for immediate start, distinct source intervals, completion-based delay, no same-source overlap/backlog, slow A while B progresses, regular collection after recoverable error, overflow/duplicate/empty/nil-runner validation.
- [ ] Write failing supervisor tests for sender-local cancellation while actual caller remains active; ErrUnavailable; ErrFatal wrapped/joined with sender timeout and caller cancellation; fatal result after a sibling reports cancellation; original causes retained; canceled timers/loops all joined before return.
- [ ] Run focused tests to observe RED, then implement one loop per source and a derived shared cancel context. Validate time.Duration conversion before goroutines; use real timers in production and fake waits in tests. No StateStore/notifier/OS signal ownership in scheduler.
- [ ] Fatal-first result handling: cancel siblings, gather worker outcomes, return fatal aggregate even if external context was also canceled. Pure governing cancellation returns that context error for command exit mapping; recoverable errors are reported and next regular collection proceeds. Report callbacks must not own worker shutdown or perform external side effects.
- [ ] Test delayed fatal discovery explicitly: C's mark fails, D remains blocked, no completed fatal result yet; release D and then observe fatal supervision. No wall-clock multi-hour test.
- [ ] Run targeted runtime, runtime race, existing Pipeline regressions and full suite; record RED/GREEN and commit for independent review.
- [ ] After review, Parent commits a concise integration handoff containing actual interfaces/commits/tests/review state and exact next steps. The next bounded daemon task is implemented by a fresh-context executor using tracked artifacts, not this conversation. Label rehearsal versus genuine new-session execution honestly.

## Task 3: Runnable monitor command and shutdown integration

**Files:** create cmd/signalwatch/main.go, focused assembly/lifecycle files and tests; create/update doc/handoffs/phase2-monitoring-mvp.md; update affected runtime status documentation. Keep existing Query CLI/MCP and Pipeline APIs intact.

**Interfaces:** command parses --sources-dir and --state-dir=/state, existing SIGNALWATCH_TELEGRAM_BOT_TOKEN/CHAT_ID env. Compose one shared SourceRunner, state.OpenSQLite, one Telegram notifier and one Pipeline per source, then runtime.Run. Keep a testable unexported run(args, getenv, streams/dependencies) entrypoint and bounded signal lifecycle; SDK types remain in Telegram package. Use a fixed internal destination ID.

- [ ] Start only from tracked checkpoint, instructions and code; record missing handoff context/questions before adding behavior. Write failing command tests for flags/env/catalog, empty/duplicate/invalid settings/interval overflow, and startup order: validate config, acquire state, then notifier; no scheduling after startup cancellation.
- [ ] Write failing lifecycle tests for signal cancellation, timer/request shutdown, pure signal exit 0, fatal+signal and cleanup errors nonzero, closing store only after workers finish, and 10-second noncooperative timeout exit. Use subprocess/helper barriers; keep production grace fixed, private test clocks may shorten test latency.
- [ ] Run focused tests RED, then implement assembly reusing existing repository/decoder/schema/catalog and native HTML registry. Send wrapper derives/cancels a 20-second child per send; mark keeps original caller context. SDK getMe retains its documented 5-second initialization bound; check cancellation immediately afterward.
- [ ] Test baseline A/B, subsequent C notification, reopen suppression and query coexistence with real SQLite, local HTML and real renderer. Use test dependencies at assembly/infrastructure seams without public endpoint override. Preserve cancellation/mark ordering, fatal aggregation and token-safe logging.
- [ ] Emit bounded JSON operational startup and source_run_completed events with source_id and success/error after RunOnce returns. This is the normal runtime report, not a test endpoint. Redact bot tokens and Telegram request URLs before error output. Acceptance may capture a log cursor while a fixture holds the response, release it, then await the next successful completion; no extra product sequence/configuration is needed.
- [ ] Add synchronized pipeline/store subprocess test for successful-send-return before mark mutation, then kill/reopen and prove duplicate eligibility; identify it as helper-tier evidence, not a production-image observation.
- [ ] Run targeted command/application/query tests, race and full suite; record actual fresh-context execution/rework evidence in handoff, commit and independently review.

## Task 4: Compose runtime, production-image acceptance and delivery

**Files:** create Dockerfile.monitor, compose.monitor.yaml, integration/monitor fixtures/controller/acceptance scripts and focused tests; modify .dockerignore, .github/workflows/ci.yaml, README.md, relevant architecture/state/notification/CI docs and final handoff. Do not modify existing tunnel runtime behavior.

**Interfaces:** one monitor service with compiled Go binary/CA certificates/non-root user/read-only sources and root FS/writable owned named state directory/init/restart policy/15-second stop grace. Dedicated test overlay/fixture image and internal network; host Compose controls containers without host Go or Docker socket inside monitor.

- [ ] Add executable failing offline acceptance assertions before runtime packaging: startup baseline A/B silent, retained volume with recreated container sees C once, repeated restart suppresses C, empty baseline survives restart, failed C/successful D retry and state suppression, independent source progress, graceful stop, second owner before getMe, fresh-volume UID and ordinary committed-state SIGKILL recovery.
- [ ] Build unchanged production monitor image and test fixture/control image. Telegram stub owns api.telegram.org DNS alias and uses an ephemeral matching TLS CA supplied solely via test overlay SSL_CERT_FILE. Exercise actual getMe/sendMessage with synthetic credentials and real renderer; deny external egress and host proxies. Do not introduce app endpoint/fake-mode flags or modify production binary for acceptance.
- [ ] Drive assertions through bounded observable barriers/control commands, not guessed sleeps; isolate project/volume names. Separate build network/image retrieval from offline runtime tests. For uncertain send, withhold response after stub acceptance and label it uncertain rather than proven post-send/pre-mark. Deterministic internal crash windows remain helper tier from Tasks 1/3.
- [ ] Use one test fixture for HTML/TLS/control and a profile-gated stopped-volume read-only inspector. Gate getMe on ready CA/listeners; never regenerate the CA while monitor runs. The real image must initialize fresh named-volume ownership, never a root test-helper chown. Assert resolved internal network, explicit empty upper/lower proxy env and literal synthetic credentials; --env-file alone does not erase shell/client injection. Disable automatic restart only in acceptance overlay so kill/recreate is controlled. If logs --follow is used for barriers, terminate/reap its host process on every path. Inspector never repairs/creates schema or reads a live writable monitor DB.
- [ ] Add bounded required Monitoring acceptance CI job while retaining Test and vet; no branch-protection changes. Use supported permission escalation if sandbox blocks authorized Docker; never weaken socket permissions. Record actual commands/exit results/image+commit+architecture; no inferred pass from syntax or skipped tests.
- [ ] Run uncached full tests, CGO/GCC race tests, vet, actual Compose acceptance, Linux arm64/amd64 builds and local target execution. Review affected docs for restart/failure/operator assumptions and credential hygiene.
- [ ] Commit and independently review task; then obtain fresh whole-branch review of spec plus complete branch and resolve material findings through applicable skills/gate.
- [ ] Open draft PR with Closes #31 and validation. Treat Actions asynchronously: no watch commands, perform independent handoff/evidence work while pending, then verify expected checks for the current PR head. Any push invalidates prior-head evidence.
- [ ] Refresh tracked handoff and report four-goal evidence, residual risks, actual/unverified checks and PR/branch. Do not merge/deploy or send real Telegram notifications.
