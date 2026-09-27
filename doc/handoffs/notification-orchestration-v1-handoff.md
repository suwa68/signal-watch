# Handoff: Notification Orchestration v1

- Project: **SignalWatch**
- Repository: https://github.com/suwa68/signal-watch
- Prepared: **2026-09-27, Asia/Taipei**
- Reviewed baseline: `7c41551e5fe5030cc647104dc3fe4bfbdc626ac1`
- Intended recipient: implementation subagent
- Status: **implementation assignment; the acceptance checkboxes below are not completion claims**

## 1. Assignment

Implement the smallest complete flow that connects the existing source collector,
in-memory baseline/deduplication processor, and Telegram destination.

The user-visible result is:

> The first successful collection establishes a silent baseline. Later newly
> observed items produce Telegram notifications. A normal unseen item becomes
> seen only after its notification succeeds; baseline items are recorded silently.

Deliver this in two ordered PRs:

1. **Phase A — Optional Telegram summary:** complete the previously discussed
   Telegram fallback adjustment.
2. **Phase B — Notification orchestration:** connect collection, baseline/dedup,
   notification construction, delivery, and the final seen-state write.

Implement both phases when dispatched. Phase A is a prerequisite within this
assignment, not the entire assignment. If Phase A is already implemented on the
current branch/main, verify and reuse it instead of creating duplicate work.

This iteration sends notifications without an NLP summary. It establishes the
delivery path that a later NLP integration can enrich.

## 2. Verified starting point

These facts were checked against the reviewed baseline. Inspect current `main`
before implementation; do not reset the repository to this commit.

| Area | Implemented behavior |
|---|---|
| Go Source v1 | File source definitions, YAML decoding, schema validation, adapter registry, source runner, native HTML collection |
| Telegram v1 | Transport-independent notification model, HTML renderer, one-attempt sender, explicit destination, manual smoke command |
| In-memory state | Deterministic item keys, source-scoped seen state, concurrency-safe memory store, atomic first-success baseline |
| Baseline/dedup processor | Returns later unseen items with their keys; does not mark normal unseen items seen |
| Missing connection | No application flow currently converts collected unseen items into notifications and marks successful deliveries |
| Known prerequisite | `Summary` is still required by the notification contract and Telegram renderer |

PR [#6](https://github.com/suwa68/signal-watch/pull/6),
“feat: add in-memory item state and baseline handling,” was merged on
2026-09-16 and closed issue [#5](https://github.com/suwa68/signal-watch/issues/5).
Its merge commit is the reviewed baseline above.

Do not use the older 2026-09-15 `project-status` document as an implementation
inventory: Telegram and in-memory state have advanced since that document.

### Required reading

Read repository instructions and the current versions of:

- `AGENTS.md`
- `README.md`
- `doc/architecture.md`
- `doc/implementation-scope.md`
- `doc/source-architecture.md`
- `doc/telegram-v1.md`
- `doc/handoffs/memory-state-baseline-v1.md`
- Relevant implementation and tests under `internal/`

The existing separate handoff, `telegram-summary-fallback-adjustment.md`, is
incorporated into Phase A below. No separate attachment is required.

This assignment deliberately advances the documented “future notification
orchestration” boundary and changes summary validation. Update those specific
status statements as implementation lands. Preserve unrelated scope boundaries.

## 3. Scope and decisions for this assignment

The following are implementation decisions for this new slice, not claims that
the repository already behaves this way.

| Concern | Decision |
|---|---|
| Source | Use the existing Go HTML adapter; keep orchestration adapter-neutral |
| Delivery | One explicitly bound Telegram destination per pipeline |
| Notification content | Original title, source name, optional original URL and publication time; empty summary |
| State | Reuse one `MemoryStateStore` across repeated runs in the same process |
| Entry point | An application-level `RunOnce(ctx, sourceConfig)` API |
| Processing | Preserve processor order; process unseen items sequentially |
| Overlapping runs | Reject overlap on the same pipeline instance before collection |
| Item failure | Leave that item unseen, continue other items while the caller context is active, return an aggregate error |
| Delivery success | Call `MarkItemSeen` using the key returned by the processor |
| Retry | No retry loop inside `RunOnce`; a later caller-driven run can encounter the item again |
| Runtime | No scheduler or full daemon in this slice |
| Source examples | Include only non-sensitive, reproducible source definitions in this public repository |

### Non-goals

Do not add:

- NLP providers, prompts, models, summary generation, or recommendation logic.
- External adapter processes, stdin/stdout protocol, Python/Java runtimes, or
  new source types.
- SQLite, PostgreSQL, Redis, file-backed state, or migrations.
- An outbox, durable queue, persisted item snapshots, or delivery history.
- Automatic backoff, retry timers, or a scheduler.
- Multiple-destination fan-out, routing rules, or destination-specific state.
- UPDATE/DELETE/MISSING semantics, content diffs, or new item identity rules.
- A global configuration redesign, new source YAML fields, or a web UI.
- Live website dependencies or real Telegram sends in automated tests.

The existing `IntervalSeconds` setting does not schedule anything in this task.

## 4. Repository workflow and PR dependency

Follow current `AGENTS.md`. Check for existing issues and branches first. Preserve
unrelated working-tree changes.

| Phase | Suggested issue title | Suggested branch |
|---|---|---|
| A | Allow Telegram fallback notifications without NLP summary | `feat/telegram-optional-summary` |
| B | Connect source collection to Telegram notification delivery | `feat/notification-orchestration-v1` |

Each issue must explain the problem, scope, non-goals, and acceptance criteria.
Each PR must reference its own issue with `Closes #<actual-number>` and report
validation results. Do not invent issue numbers.

Implement Phase A before Phase B:

- If A is already merged, branch B from current `main`.
- If A is awaiting review, this assignment permits B to branch from A and open a
  dependent PR targeting A's branch. Clearly state the dependency in both PRs.
- After A is merged, rebase B onto the updated `main`, retarget the PR, and check
  that B's diff contains only its own changes. Account for squash merging.
- Follow the repository's review/merge policy. A pending review does not prevent
  implementing and testing the dependent work.

Do not commit feature implementation directly to `main`. Use the repository's
actual documentation directory, `doc/`.

## 5. Phase A — Optional Telegram summary

### 5.1 Contract

Keep the existing struct shape:

```go
type Notification struct {
    Title       string
    Summary     string
    SourceName  string
    URL         string
    PublishedAt *time.Time
}
```

Change its semantics:

| Field | Requirement |
|---|---|
| `Title` | Required; nonblank, valid UTF-8 |
| `SourceName` | Required; nonblank, valid UTF-8 |
| `Summary` | Optional; empty/whitespace-only means absent; supplied non-empty text must be valid UTF-8 |
| `URL` | Optional; preserve existing URL validation |
| `PublishedAt` | Optional; preserve the provided timestamp and location |

Validate non-empty summary bytes as UTF-8 even when deciding whether the summary
is absent. Do not silently accept malformed text.

Do not make title or source name optional. Do not introduce pointers, an NLP
result envelope, or a fallback-status field merely to represent an absent summary.

### 5.2 Rendering

When a summary is present, preserve current rendering.

When it is absent, omit the summary section completely. For example, the
visible output with a URL should be:

```text
🔔 Example announcement

Source: Example News

View original
```

The final line remains the existing HTML link to the original URL. Without a
URL, omit that line. With a publication time, retain the existing timestamp line.

Do not add placeholder text, an empty summary section, or messages such as
“AI summary failed.” Telegram rendering must not depend on why a summary is absent.

Preserve:

- HTML escaping for external text and URL attributes.
- Existing absolute HTTP/HTTPS URL validation.
- HTML parse mode and disabled link previews.
- One message per notification and one send attempt.
- Existing SDK, token initialization, and destination behavior.

### 5.3 Length handling

The existing budget is **3800 UTF-16 code units of visible rendered text**.
Keep its counting rules, including line breaks, labels, and non-BMP characters.

With a summary:

- Preserve title and metadata.
- Truncate only the summary, before escaping.
- Preserve the existing ellipsis and insufficient-space behavior.

Without a summary:

- Measure the actual title-plus-footer message.
- Accept it when it fits the budget, including the exact-limit case.
- Reject it only when that actual message exceeds the budget.
- Do not reserve a summary character or manufacture an ellipsis.

### 5.4 Expected changes

At minimum inspect and update:

- `internal/notification/model.go`
- `internal/notification/telegram/renderer.go`
- Relevant renderer and notifier tests
- `doc/telegram-v1.md`
- `doc/architecture.md`
- Current comments/docs that claim summary is required

The notifier should require little or no functional change. The existing
Telegram smoke command may retain its non-empty summary.

## 6. Phase B — Application orchestration

### 6.1 Reuse existing components

The reviewed repository exposes:

```go
// internal/source
func (*SourceRunner) Run(
    ctx context.Context,
    sourceConfig config.SourceConfig,
) ([]source.MonitorItem, error)

// internal/monitoring
type UnseenItem struct {
    Item source.MonitorItem
    Key  string
}

func (*Processor) ProcessSuccessfulCollection(
    ctx context.Context,
    sourceID string,
    items []source.MonitorItem,
) ([]monitoring.UnseenItem, error)

// internal/state
type StateStore interface {
    IsSourceInitialized(context.Context, string) (bool, error)
    EstablishBaseline(context.Context, string, []string) (bool, error)
    HasItem(context.Context, string, string) (bool, error)
    MarkItemSeen(context.Context, string, string) error
}

// internal/notification/telegram
func (*Notifier) Send(
    ctx context.Context,
    destination telegram.Destination,
    message notification.Notification,
) error
```

These signatures are shown using qualified types for readability, not as a
literal source file to paste into a package.

Reuse the existing processor and item-key builder. Do not implement a second
deduplication algorithm, separate baseline flag, or second state store.

### 6.2 Package and interface direction

A small `internal/application` package is a reasonable location. Adapt names to
current repository conventions without restructuring unrelated packages.

Suggested application-facing shapes:

```go
type Collector interface {
    Run(context.Context, config.SourceConfig) ([]source.MonitorItem, error)
}

type NotificationSender interface {
    Send(context.Context, notification.Notification) error
}

type NotificationSenderFunc func(
    context.Context,
    notification.Notification,
) error

func (f NotificationSenderFunc) Send(
    ctx context.Context,
    message notification.Notification,
) error {
    return f(ctx, message)
}

// Constructor should reject missing dependencies.
func NewPipeline(
    collector Collector,
    store state.StateStore,
    sender NotificationSender,
) (*Pipeline, error)

func (*Pipeline) RunOnce(
    ctx context.Context,
    sourceConfig config.SourceConfig,
) error
```

`NewPipeline` should create/reuse a `monitoring.Processor` with the **same store**
that the pipeline uses for `MarkItemSeen`. Do not accidentally inject one store
into discovery and another into completion.

Bind Telegram at application assembly:

```go
// Illustrative assembly: handle construction errors in real code.
store := state.NewMemoryStateStore()

sender := application.NotificationSenderFunc(
    func(ctx context.Context, message notification.Notification) error {
        return telegramNotifier.Send(ctx, destination, message)
    },
)

pipeline, err := application.NewPipeline(sourceRunner, store, sender)
if err != nil {
    return err
}

// Reuse pipeline, store, notifier, and destination for every run.
// The caller decides when to invoke the next run.
err = pipeline.RunOnce(ctx, sourceConfig)
```

The assembly provides an already-initialized notifier and a valid explicit
destination. Do not call `telegram.New` for every item or every run.

The orchestration algorithm must not import Telegram SDK types. A bound
function adapter is sufficient; do not create a routing framework or replace
the existing Telegram API.

### 6.3 Single-run algorithm

Implement the following order:

1. Reject an already-canceled context and invalid required source metadata.
   Require a nonblank source ID and a nonblank, valid UTF-8 source name.
2. Acquire the pipeline's non-overlap guard. If another call is active, return
   an `errors.Is`-compatible `ErrRunInProgress` before collection or side effects.
   Release the guard on every exit.
3. Call the existing collector with the caller's context and source config.
4. If collection returns any error, return it with source/stage context.
   **Do not call the processor**, even if the collector also returned items.
5. Pass only successful collection results to
   `ProcessSuccessfulCollection(ctx, sourceConfig.ID, items)`.
6. If processing fails, return the error; do not deliver items from that failed
   processing attempt.
7. For each returned unseen item, in the processor's order:
   - Inspect the caller's `ctx.Err()` before starting another item. If non-nil,
     stop and return that caller context error together with accumulated failures.
   - Build its notification using the mapping below.
   - Send the notification once through the bound sender. If sending fails,
     retain the item error and leave the item unseen. A sender-local timeout or
     cancellation error does not stop later items while `ctx.Err() == nil`.
   - Only if sending returns nil, call
     `MarkItemSeen(ctx, sourceConfig.ID, unseen.Key)`.
8. Preserve item failures and return an aggregate error after other eligible
   items have been attempted. Inspect `ctx.Err()` after item work and before
   returning, including after the final item. If non-nil, join the caller context
   error with accumulated failures. Do not use `errors.Is(sendErr,
   context.DeadlineExceeded)` or `errors.Is(sendErr, context.Canceled)` alone to
   decide whether the run must stop.

The existing processor already handles baseline creation and duplicate keys
within one collection. Do not pre-mark new items or recompute their keys.

The non-overlap guard is local to one pipeline instance. This slice uses one
pipeline for the application's runs. It does not coordinate separately
constructed pipelines or different processes.

### 6.4 Notification mapping

| Notification field | Source |
|---|---|
| `Title` | `unseen.Item.Title` |
| `Summary` | Empty string |
| `SourceName` | `sourceConfig.Name` |
| `URL` | `unseen.Item.URL` |
| `PublishedAt` | `unseen.Item.PublishedAt` |

Validate the mapped required title/source text. A malformed unseen item produces
a mapping error and remains unseen. Do not fabricate a title from an ID.

Additional rules:

- Do not copy `Content` into `Summary` as a substitute for NLP.
- Do not duplicate the title inside `Summary` to bypass validation.
- Do not HTML-escape in the mapper; rendering owns escaping.
- A missing URL is valid. Invalid supplied URLs remain subject to the existing
  renderer's validation.
- Preserve source-provided publication time; do not replace it with “now.”
- Map only items returned as unseen. Do not tighten baseline item validation
  beyond the existing processor's identity rules.
- Scope state writes using the configured source ID and the processor's key.

A small pure helper for mapping is appropriate. No content-processor interface
or fake NLP provider is required.

### 6.5 Error and completion semantics

| Failure/success | Seen-state behavior | Remaining work in this run |
|---|---|---|
| Source config invalid | No collection or state mutation | Return error |
| Collection fails, including partial items + error | No baseline or item mutation by orchestration | Return error |
| First successful collection is empty | Source initializes through the processor | No notifications |
| Item identity/processor error | Preserve existing processor semantics; no delivery | Return error |
| Notification mapping fails | Failed item remains unseen | Continue later items |
| Renderer or sender fails, including sender-local timeout/cancellation while `ctx.Err() == nil` | Failed item remains unseen | Retain the error and continue later items |
| Send succeeds and `MarkItemSeen` succeeds | Item becomes seen | Continue |
| Send succeeds but `MarkItemSeen` fails | No successful completion can be claimed for that item | Continue, return the state error |
| Caller context canceled/deadline exceeded (`ctx.Err() != nil`) | No forced state write with a new background context | Stop starting new items and join the caller context error with accumulated failures |

Use `%w` and, where appropriate, `errors.Join` so `errors.Is` / `errors.As`
continue to work. Include source ID, item key when available, and failing stage.
The context passed to `RunOnce` is the authority for stopping the run. Preserve
all accumulated item errors when caller cancellation ends a partially completed
run. A sender may use its own child context or timeout; its error alone is not
proof that the caller canceled the run.

A successful send followed by a failed state write may lead to a duplicate on
a later run. Accept that possibility; do not turn a failed state write into
success or roll back earlier successful items.

A send error can also mean that delivery outcome is uncertain. Do not claim
exactly-once delivery or try to infer remote success from an error.

If all attempted work succeeds, return nil. Baseline-only and no-new-item runs
are successful no-op delivery runs.

### 6.6 Lifetime, retries, and limits

These constraints must be documented and tested where applicable:

- The memory store belongs to the running application, not to a single call.
- A new store/pipeline on every invocation would baseline every collection and
  suppress all notifications. Do not implement the entry point that way.
- A later attempt is possible only if the item is collected again with the same
  identity while the relevant state is available.
- There is no durable pending-item snapshot. An item that disappears from the
  source after a failed send is not guaranteed to be recovered.
- Process restart loses baseline and seen state. A new process establishes a
  new silent baseline; restart continuity is not guaranteed.
- State is scoped by source and item, not destination. Keep the destination
  binding stable for the pipeline lifetime. Dynamic destination changes and
  independent delivery tracking for multiple destinations are out of scope.
- A concurrency-safe store alone does not prevent duplicate concurrent sends;
  retain the pipeline overlap guard.
- Do not describe this slice as durable at-least-once delivery.

### 6.7 Entry point and demonstration

A reusable Go API plus deterministic integration tests and an assembly example
is sufficient for this slice. A production daemon or polling CLI is not required.

Do not present multiple executions of a fresh one-shot process as a working
monitoring loop: process-local state would reset on each execution.

Provide a documented local integration-test command that demonstrates repeated
`RunOnce` calls in one process and uses no credentials. If an extra example
command is added, keep it local/deterministic and small; it must not introduce a
scheduler or silently send real Telegram messages.

## 7. Required verification

Use existing fixtures, fakes, and local `httptest.Server` patterns. Keep tests
focused on the new behavior and regression risks; reuse existing coverage for
unchanged source extraction and key generation.

### 7.1 Phase A tests

| Case | Expected result |
|---|---|
| Existing non-empty summary | Existing rich format and truncation still work |
| Empty summary | Accepted; summary section omitted |
| Whitespace-only summary | Treated as absent |
| No summary with URL | Correct title/source/link output |
| No summary without URL | Correct title/source output |
| Optional publication time | Correct presence/absence and existing location policy |
| External text/URL special characters | Existing escaping remains correct |
| Invalid UTF-8 non-empty summary | Rejected |
| Missing/invalid required title or source name | Rejected |
| No-summary visible text exactly at the budget | Accepted |
| No-summary visible text over the budget | Rejected before sending |
| CJK/non-BMP text at relevant boundaries | Existing UTF-16 counting remains correct |
| No-summary notification through notifier | Exactly one send, expected HTML mode, previews disabled |

Use the Telegram package's existing private fake/local HTTP seams. Do not export
SDK test hooks or contact real Telegram to make these tests possible.

### 7.2 Orchestration tests

| Case | Required evidence |
|---|---|
| First success: A, B | Baseline established; zero sends |
| Next success: A, B, C | C sent once; C seen only after send succeeds |
| Third success: A, B, C | Zero additional sends |
| First success empty, next success C | Empty baseline initializes; C is delivered |
| First collection fails | Source remains uninitialized; next success establishes baseline |
| Collector returns items and error | Processor is bypassed; no baseline/send |
| Invalid identity in a baseline batch | No partial baseline; no sends |
| Duplicate new keys in one batch | One send for that key |
| Mapping without NLP | Exact field mapping; Summary remains empty even if Content exists |
| Missing optional URL/time | Notification remains valid |
| Unseen item lacks a valid title | Item not sent/marked; later valid item still attempted |
| Send fails for C, succeeds for D | C remains unseen; D is marked; run returns error |
| Later collection includes C and D | C can be attempted again; D is suppressed |
| State write fails after successful send | Error returned; success is not falsely reported; later repetition is allowed |
| Send/mark ordering | Sender observes the item still unseen at send time; marking follows success |
| Sender-local timeout with active caller | After baseline, C returns a wrapped `context.DeadlineExceeded`; C stays unseen, D is sent and marked, aggregate preserves C's failure |
| Sender-local cancellation with active caller | Table-driven variant using wrapped `context.Canceled`; C stays unseen and D still completes |
| Actual caller cancellation during failed send | Cancel caller during C's failed send; D is neither sent nor marked; aggregate preserves C's failure, earlier item failures, and caller cancellation |
| Caller context cancellation | Context propagated; no further items started once `ctx.Err() != nil`; error remains detectable, including on the final item |
| Overlapping `RunOnce` calls | Second call returns `ErrRunInProgress` without collection or send |
| Guard release after an error | A subsequent non-overlapping run can execute |
| Store lifetime | Repeated runs use the same state; no per-run recreation |

Use a controllable fake store for `MarkItemSeen` failures and a channel-based
fake collector/sender for overlap tests. Avoid sleep-based timing tests.

Existing processor tests cover source isolation. Preserve them; do not replace
them with a duplicate implementation-specific test suite.

### 7.3 Integration scenario

Add at least one deterministic integration test containing:

1. A temporary YAML source definition.
2. The real file repository, YAML decoder, schema validation, registry, HTML
   adapter, and `SourceRunner`.
3. A local HTML server whose content changes from A/B to A/B/C.
4. The real memory store and baseline/dedup processor.
5. The new pipeline and notification mapper.
6. The real Telegram renderer invoked by a recording sender, with no live API.
7. Repeated calls in the same process demonstrating baseline, new-item delivery,
   and later suppression.

It is acceptable for the recording sender to call `telegram.Renderer.Render`
and record the resulting HTML. Keep the actual notifier's send behavior covered
inside the Telegram package using its existing test seams, including the new
no-summary case. This avoids exposing infrastructure internals for testing.

Do not claim that this local test verifies real Telegram receipt.

### 7.4 Validation commands

Use the repository's Docker-based Go toolchain when available:

```sh
docker compose run --rm go go vet ./...
docker compose run --rm go go test ./...
docker compose run --rm go go test -race ./...
```

Format changed Go files with `gofmt`.

If Docker is unavailable but a compatible local Go toolchain is available,
use the equivalent `go vet ./...`, `go test ./...`, and `go test -race ./...`.
Report the actual toolchain and commands used.

If a required command is blocked by the environment, report the exact blocker
and any completed alternative checks. Do not mark an unexecuted test as passed.

No real bot token, destination identifier, or live send is required. Running an
existing live smoke command is a separate user-authorized operational action,
not an automated acceptance requirement.

## 8. Documentation deliverables

Update implementation status, not just design prose:

- `README.md`: explain the new callable flow, its local test command, and the
  process-local state limitation.
- `doc/architecture.md`: connect source collection, processor, notification
  construction, delivery, and successful seen marking.
- `doc/telegram-v1.md`: document optional summaries and both rendering forms.
- A focused orchestration document, such as
  `doc/notification-orchestration-v1.md`: assembly example, ordering, errors,
  caller-driven repeated runs, overlap policy, and limitations.
- Relevant comments and current scope statements in `AGENTS.md`, if the
  implemented scope changes them.

Preserve the historical scope of earlier handoffs. If a historical statement
about NLP failure conflicts with successful fallback delivery, add an explicit
current-policy note rather than implying the old implementation already handled it.

For the future NLP slice, the intended policy is: summarization failure may
produce an empty-summary notification; **successful fallback delivery can then
mark the item seen**. This assignment prepares that path but adds no NLP stage.

Do not broaden this work into external adapters, persistence, or scheduling.

## 9. Acceptance checklist

### Phase A

- [ ] Summary is optional in model documentation and renderer validation.
- [ ] Empty/whitespace-only summary produces no empty section or placeholder.
- [ ] Existing rich notifications, escaping, and truncation remain correct.
- [ ] No-summary length handling uses the actual visible message.
- [ ] Notifier tests demonstrate a no-summary send without live Telegram.
- [ ] Relevant documentation and tests are updated.

### Phase B

- [ ] One callable application flow connects the existing collector and processor to delivery.
- [ ] The existing memory store and processor are reused.
- [ ] A bound sender keeps SDK types outside the orchestration algorithm.
- [ ] New notifications use original metadata and an empty summary.
- [ ] First successful collection, including empty, is silent baseline.
- [ ] Failed collection never initializes the source.
- [ ] Normal unseen items are marked only after successful delivery.
- [ ] Mapping/send failures leave items unseen and do not block later items.
- [ ] State-write failures after a send are reported.
- [ ] The caller context is propagated; `ctx.Err() != nil` stops new items and is joined with accumulated item failures.
- [ ] Wrapped sender-local timeout/cancellation errors leave items unseen and allow later items while the caller context is active.
- [ ] Deterministic C/D regression tests cover both sender-local error variants and actual caller cancellation during C's failed send.
- [ ] Overlapping runs on one pipeline are rejected.
- [ ] Repeated same-process runs share state.
- [ ] Integration coverage demonstrates A/B → A/B/C → A/B/C behavior.
- [ ] Retry eligibility and restart/data-loss limits are documented accurately.
- [ ] No NLP provider, persistence, external adapter, scheduler, or routing framework is added.
- [ ] Relevant formatting, vet, unit/integration, and race checks are completed or precisely reported as blocked.
- [ ] Issue/branch/PR workflow is followed for both phases.

## 10. Final implementation report

Return a concise report with:

1. Phase A and Phase B completion status.
2. Issue links, branch names, PR links, and any remaining dependency between PRs.
3. Files added/changed and the purpose of the changes.
4. Final public/application interfaces.
5. Exact notification mapping and success/failure behavior.
6. Test commands, results, and any environmental blockers.
7. How to reproduce the credential-free local integration scenario.
8. Assumptions, deviations from this assignment, and unresolved questions.
9. Explicit confirmation that no live Telegram send or secret commit occurred
   during automated validation.

Do not stop after design notes when dispatched to implement this assignment.
Implement the code, tests, and documentation within the scope above, and leave
the reviewable PRs ready under the repository's normal workflow.

## 11. Source references

These links identify the reviewed baseline. Consult current versions before
editing and report meaningful drift.

- [README at reviewed baseline](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/README.md)
- [Repository instructions](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/AGENTS.md)
- [Architecture](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/doc/architecture.md)
- [Telegram guide](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/doc/telegram-v1.md)
- [Baseline/state handoff](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/doc/handoffs/memory-state-baseline-v1.md)
- [Baseline/dedup processor](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/internal/monitoring/processor.go)
- [StateStore](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/internal/state/store.go)
- [Notification model](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/internal/notification/model.go)
- [Telegram renderer](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/internal/notification/telegram/renderer.go)
- [Telegram notifier](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/internal/notification/telegram/notifier.go)
- [Existing source integration-test pattern](https://github.com/suwa68/signal-watch/blob/7c41551e5fe5030cc647104dc3fe4bfbdc626ac1/internal/source/html/pipeline_integration_test.go)
- [Issue #5](https://github.com/suwa68/signal-watch/issues/5) and [merged PR #6](https://github.com/suwa68/signal-watch/pull/6)
