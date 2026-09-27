# Notification Orchestration v1

Tracking issue: [#8 — Notification orchestration](https://github.com/suwa68/signal-watch/issues/8).
Optional summaries are the prerequisite delivered in
[PR #9](https://github.com/suwa68/signal-watch/pull/9) for
[issue #7](https://github.com/suwa68/signal-watch/issues/7).

## Scope

The application pipeline connects the existing source collector, baseline/dedup
processor, notification model, and bound sender. Its first successful collection
establishes a silent baseline. Later unseen items produce notifications using
original metadata and an empty summary; successful delivery is followed by
seen marking.

This slice is a reusable Go API. It adds no daemon, scheduler, NLP stage,
durable state, retry loop, outbox, routing framework, or external adapter.
`SourceConfig.IntervalSeconds` does not schedule calls.

## API and boundaries

The `internal/application` package exposes:

```go
type Collector interface {
    Run(context.Context, config.SourceConfig) ([]source.MonitorItem, error)
}

type NotificationSender interface {
    Send(context.Context, notification.Notification) error
}

type NotificationSenderFunc func(context.Context, notification.Notification) error

func (NotificationSenderFunc) Send(context.Context, notification.Notification) error

func NewPipeline(Collector, state.StateStore, NotificationSender) (*Pipeline, error)
func (*Pipeline) RunOnce(context.Context, config.SourceConfig) error

var ErrRunInProgress error
```

`SourceRunner` satisfies `Collector`. The constructor rejects missing
dependencies and creates the existing `monitoring.Processor` with the same store
used by `MarkItemSeen`. It does not duplicate item-key or baseline logic.

The sender contract accepts only a transport-independent notification. Bind
Telegram's destination in assembly so orchestration needs no Telegram SDK types.

## Assembly and lifetime

The following assembly function belongs inside this Go module because it uses
`internal` packages. Its token and destination arguments come from application
configuration; do not commit real values. Calling `telegram.New` initializes
the bot through Telegram's `getMe`, so this is a production assembly example,
not the credential-free test below.

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/suwa68/signal-watch/internal/application"
    "github.com/suwa68/signal-watch/internal/notification"
    "github.com/suwa68/signal-watch/internal/notification/telegram"
    "github.com/suwa68/signal-watch/internal/source"
    htmlsource "github.com/suwa68/signal-watch/internal/source/html"
    "github.com/suwa68/signal-watch/internal/state"
)

func assemblePipeline(
    token string,
    destination telegram.Destination,
) (*application.Pipeline, error) {
    if strings.TrimSpace(destination.ID) == "" ||
        strings.TrimSpace(destination.ChatID) == "" {
        return nil, fmt.Errorf("Telegram destination ID and chat ID are required")
    }

    registry := source.NewSourceAdapterRegistry()
    client := &http.Client{Timeout: 30 * time.Second}
    if err := registry.Register("html", htmlsource.New(client)); err != nil {
        return nil, fmt.Errorf("register HTML adapter: %w", err)
    }
    runner := source.NewSourceRunner(registry)

    notifier, err := telegram.New(token)
    if err != nil {
        return nil, fmt.Errorf("initialize notification delivery: %w", err)
    }
    sender := application.NotificationSenderFunc(
        func(ctx context.Context, message notification.Notification) error {
            return notifier.Send(ctx, destination, message)
        },
    )

    store := state.NewMemoryStateStore()
    pipeline, err := application.NewPipeline(runner, store, sender)
    if err != nil {
        return nil, fmt.Errorf("assemble notification pipeline: %w", err)
    }
    return pipeline, nil
}
```

Call this once when assembling the application, then retain the returned
pipeline. Load a source using the file repository, YAML decoder, and
`config.ParseSourceConfig`; pass that validated configuration and the caller's
context to each run:

```go
if err := pipeline.RunOnce(ctx, sourceConfig); err != nil {
    return fmt.Errorf("run source %q: %w", sourceConfig.ID, err)
}
```

The surrounding application handles the error and decides when to invoke
`RunOnce` again. Reuse the pipeline, store, notifier, and destination for those
calls. Creating a fresh store on each call silently baselines every collection
and suppresses new-item notifications. Multiple executions of a fresh one-shot
process cannot demonstrate a working monitoring loop.

One pipeline binds one stable destination. The notifier itself supports
multiple destinations, but state is scoped by source/item rather than
destination. Changing the binding during the pipeline lifetime or tracking
independent delivery to several destinations is outside this slice.

## Run ordering

1. Reject an already-canceled caller context and invalid source metadata.
   Source ID must be nonblank; source name must be nonblank, valid UTF-8.
2. Acquire the pipeline's non-overlap guard. An overlapping call returns an
   `errors.Is`-compatible `ErrRunInProgress` before collection or state mutation.
   Every exit releases the guard.
3. Collect with the caller's context. Any collection error, including partial
   items accompanied by an error, bypasses the processor.
4. Process the successful collection with the existing processor and configured
   source ID. The first success, including an empty result, establishes an
   atomic silent baseline. A processor error prevents delivery in that attempt.
5. Visit returned unseen items sequentially in processor order. Inspect
   `ctx.Err()` before starting each item; stop when it is non-nil.
6. Map the item, send once, and only after a nil send error call
   `MarkItemSeen(ctx, sourceConfig.ID, unseen.Key)` using the processor's key.
   Retain mapping, send, or mark failures and continue while the caller context
   remains active. Do not recompute keys or pre-mark unseen items.
7. Inspect the caller context after item work and before returning, including
   after the final item. Join its error with accumulated failures. Return nil
   when all work succeeds, including baseline-only and no-new-item runs.

The overlap guard applies to one pipeline instance, including calls for
different sources. It does not coordinate independently constructed pipelines
or different processes. A concurrency-safe store alone does not serialize sends.

## Notification mapping

| Notification field | Value |
|---|---|
| `Title` | `unseen.Item.Title` |
| `Summary` | Empty string |
| `SourceName` | `sourceConfig.Name` |
| `URL` | `unseen.Item.URL` |
| `PublishedAt` | `unseen.Item.PublishedAt` |

Required title and source text must be nonblank and valid UTF-8. Malformed unseen
items fail mapping without inventing a title. Baseline items keep the existing
processor's identity validation and are not mapped for delivery. Missing URL or
publication time is valid; supplied URLs are validated by the Telegram renderer.
Publication time retains the source's timestamp and location.

The mapper copies neither `Content` nor the title into `Summary`, and does not
HTML-escape text. Rendering owns escaping and optional-summary formatting. No
NLP provider or fallback-status field is involved. A future summarization
failure may use an empty-summary fallback; successful fallback delivery can
then mark the item seen.

## Errors and caller cancellation

| Result | State and remaining work |
|---|---|
| Invalid source metadata | No collection or state mutation; return error |
| Collection failure, even with partial items | No processing, baseline, or delivery; return error |
| Processor failure | No delivery; retain existing processor state semantics |
| Mapping or send failure | Leave item unseen, retain error, attempt later items while caller is active |
| Successful send and successful mark | Item becomes seen; continue |
| Successful send but failed mark | Retain state error, claim no successful completion for the item, continue while caller is active |
| Caller `ctx.Err() != nil` | Stop starting items; return caller error joined with accumulated item failures |

The context passed by the caller to `RunOnce` is the sole authority for stopping
new items. A sender may use its own child context or timeout. If it returns a
wrapped `context.DeadlineExceeded` or `context.Canceled` while `ctx.Err() == nil`,
that is an item-level failure: retain it, leave that item unseen, and continue.
Do not infer run cancellation solely from `errors.Is(sendErr, ...)`.

Errors retain source ID, item key when available, and failing stage using `%w`
and aggregate errors so callers can use `errors.Is` and `errors.As`. For example,
an aggregate matching `context.DeadlineExceeded` may contain a sender-local
timeout even though the caller context remains active. Inspect the caller
context to distinguish those situations. Caller cancellation during the final
item is also preserved; there need not be a later item to detect it.

Seen writes use the caller's context. Cancellation does not trigger a forced
write using `context.Background()`, and earlier successful items are not rolled
back. A successful send followed by a failed mark may yield a duplicate on a
later run. A send error can itself mean the remote delivery outcome is unknown.

## Retry and state limits

There is one send attempt per unseen item per run and no automatic retry timer.
A later caller-driven run can retry only when the item is collected again with
the same identity while relevant state is available. Failed items are not saved
as durable pending snapshots; disappearance from the source can prevent retry.

Process restart loses baseline and seen state. The new process establishes a
new silent baseline, so restart continuity and recovery of pending deliveries
are not guaranteed. Duplicate delivery is possible, but this is neither
exactly-once nor durable at-least-once delivery. No update/deletion semantics,
content diffs, or destination-specific state are added.

## Local verification

Run the deterministic, credential-free integration scenario:

```sh
docker compose run --rm go go test ./internal/application -run TestPipelineFromYAMLToNotifications -v
```

The scenario uses a temporary YAML definition, real file repository and decoder,
schema validation, registry, HTML adapter, source runner, memory state,
baseline/dedup processor, pipeline, and real Telegram renderer. A local HTML
server changes from A/B to A/B/C. Repeated calls in one process establish the
baseline, record a notification for C, and suppress it on the next call.
A recording sender renders HTML without contacting Telegram; this test does
not verify real Telegram receipt.

The deterministic cancellation regressions establish a baseline and then
process C and D:

- A table-driven sender-local timeout/cancellation case returns a wrapped
  `context.DeadlineExceeded` or `context.Canceled` for C while the caller remains
  active. C remains unseen, D is sent and marked, and the aggregate retains C's
  failure.
- Actual caller cancellation during C's failed send prevents D from being sent
  or marked. The returned error preserves caller cancellation and accumulated
  item failures.

Other regression cases cover successful-send-before-mark ordering, collection
and mapping errors, failed completion writes, empty baselines, duplicates,
guard release and overlap rejection without timing sleeps, and shared state
across calls. Existing Telegram package tests cover actual notifier behavior
with private fakes/local HTTP seams.

Full validation commands:

```sh
docker compose run --rm go go vet ./...
docker compose run --rm go go test ./...
docker compose run --rm go go test -race ./...
```
