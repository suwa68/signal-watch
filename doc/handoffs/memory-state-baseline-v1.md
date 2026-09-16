# Handoff: In-Memory State and Baseline v1

## Objective

Implement the smallest useful item-state layer for **SignalWatch v1**.

This iteration exists to answer only two runtime questions:

1. Has this source already completed its initial baseline?
2. Has this item already been seen for this source?

The target flow is:

```text
SourceRunner
    |
    v
MonitorItem[]
    |
    v
BuildItemKey
    |
    v
StateStore
    |
    +--> already seen -> ignore
    |
    +--> unseen -> downstream processing
```

For the first successful collection of a source:

```text
First successful source collection
        |
        v
all currently visible items
        |
        v
mark items as seen
        |
        v
NO NLP
NO notification
        |
        v
mark source initialized
```

This iteration is intentionally **memory-only**.

Do not add SQLite, PostgreSQL, Redis, files, or any other durable persistence backend.

---

## Agreed v1 Product Semantics

### Seen vs Unseen only

SignalWatch v1 does **not** implement item lifecycle tracking.

Do not implement:

- UPDATED
- DELETED
- MISSING
- item version history
- content diffing
- change-event state machines

The only v1 question is:

> Has this item already been seen for this source?

### Duplicate delivery is acceptable

SignalWatch v1 prefers:

> duplicate notification over missing notification.

The intended behavior is approximately **at-least-once**.

If downstream notification succeeds and SignalWatch crashes before recording the item as seen, the item may be processed and delivered again later.

That is acceptable.

Do not optimize for exactly-once behavior.

### Memory-only state is acceptable for v1

The first implementation may lose all state when the SignalWatch process restarts.

Restart continuity is therefore not guaranteed.

That tradeoff is accepted for the current version.

The API should still use a small interface so a future durable store can be added without rewriting application flow.

---

## Current MonitorItem Assumptions

Use the repository's existing model if it already exists.

The currently discussed provisional model is equivalent to:

```go
type MonitorItem struct {
    SourceID   string
    ExternalID string

    Title   string
    Content string
    URL     string

    PublishedAt *time.Time
}
```

Semantics:

- `SourceID` identifies the SignalWatch source.
- `ExternalID` is optional.
- `ExternalID` must represent an identifier supplied by the source.
- SignalWatch must not fabricate `ExternalID`.
- `URL` is optional.
- `PublishedAt` is optional and source-provided.
- sync/observation time is runtime metadata and is not source identity.

Do not permanently redesign `MonitorItem` in this task.

---

## Item Key Strategy

Implement a small deterministic item-key builder.

Fallback order:

```text
1. ExternalID
2. URL
3. PublishedAt + Title
4. content-derived key from Title + Content
```

The effective identity is always scoped by `SourceID`.

Examples:

```text
external:123
url:https://example.com/news/123
published:2026-09-15T10:30:00Z:title:MLCC Update
content:<sha256>
```

The store should use:

```text
SourceID + ItemKey
```

as the effective uniqueness scope.

### URL handling

Do not build a complex canonical URL system.

Use the URL supplied by the adapter, assuming relative URLs were already resolved by the source layer.

Do not introduce:

- tracking-parameter stripping
- redirect resolution
- canonical-link discovery
- generic URL normalization frameworks

### Content-derived fallback

If no `ExternalID`, `URL`, or usable `PublishedAt + Title` exists, derive a deterministic key from:

```text
Title + Content
```

Use a stable hash such as SHA-256.

Do not use current/sync time as the primary deduplication key.

If no deterministic fallback is possible, return a clear error.

An item has no deterministic fallback when both `Title` and `Content` are
blank. That error fails the current source-processing attempt, not the whole
SignalWatch process. Build and validate every item key before changing baseline
state so one invalid item cannot leave a partial baseline.

---

## StateStore Contract

Keep the interface minimal.

Conceptually:

```go
type StateStore interface {
    IsSourceInitialized(
        ctx context.Context,
        sourceID string,
    ) (bool, error)

    EstablishBaseline(
        ctx context.Context,
        sourceID string,
        itemKeys []string,
    ) (established bool, err error)

    HasItem(
        ctx context.Context,
        sourceID string,
        itemKey string,
    ) (bool, error)

    MarkItemSeen(
        ctx context.Context,
        sourceID string,
        itemKey string,
    ) error
}
```

Exact names may be adjusted to existing project conventions.

`EstablishBaseline` is a domain-specific atomic operation, not a generic
transaction API. If the source is not initialized, it records all supplied
keys and source initialization as one operation and returns `true`. If another
caller already initialized the source, it makes no changes and returns `false`.
An empty item-key slice still initializes the source.

Do not add generic CRUD methods.

Do not add APIs for updates, deletes, searches, history, or generic
transactions.

---

## MemoryStateStore

Implement:

```text
MemoryStateStore
```

It must be safe for concurrent access.

A simple implementation may use:

- maps
- `sync.RWMutex`

Conceptually:

```text
initializedSources map[string]struct{}
seenItems          map[string]map[string]struct{}
```

No external cache or database.

---

## Baseline Semantics

### First successful collection

The first successful collection for each source establishes that source's baseline.

Example:

```text
Source starts for the first time
        |
        v
collect succeeds
        |
        v
100 items returned
        |
        v
all item keys and source initialization
committed atomically
        |
        v
NO NLP
NO Telegram
```

These currently visible items are historical baseline data and must not enter downstream processing.

Every item key must be built successfully before `EstablishBaseline` is called.
If any item has no deterministic identity, the current source-processing
attempt fails and baseline state remains unchanged.

### Empty first successful collection

If the first successful collection returns zero items:

```text
collect success
items = []
```

the source must still be marked initialized.

Do not infer first-run status from whether the seen-item store is empty.

Otherwise, the first future item could be swallowed as baseline.

### Failed first collection

A failed collection must **not** initialize the source.

Baseline completes only after a successful collection.

---

## Normal Runtime Semantics

After a source is initialized:

```text
collect
  |
  v
MonitorItem[]
  |
  v
for each item:
  |
  v
BuildItemKey
  |
  v
HasItem?
```

If already seen:

```text
ignore
```

Do not run NLP.

Do not notify.

If unseen:

```text
forward to downstream processing
```

This task does not implement NLP or Telegram.

---

## Critical Processing Order

Do **not** mark an unseen item as seen immediately after discovery.

The agreed future orchestration is:

```text
unseen item
    |
    v
NLP processing
    |
    v
Telegram notification
    |
    v
notification succeeds
    |
    v
MarkItemSeen
```

If NLP or notification fails:

```text
do not MarkItemSeen
```

The item should remain eligible for reprocessing.

If notification succeeds but the process crashes before `MarkItemSeen`, a duplicate notification may occur later.

That duplicate is acceptable.

---

## Scope Boundary

This task should not implement the complete end-to-end pipeline.

Implement state primitives and a small baseline/dedup processor for successful
collections.

Do not introduce NLP or Telegram dependencies into the state package.

The application/orchestration layer will decide when to call `MarkItemSeen`.

The processor receives only successful collection results. A collection error
must bypass it, so a failed collection cannot initialize the source.

---

## Concurrency Requirements

The in-memory implementation must be safe when accessed from multiple goroutines.

At minimum:

- concurrent `HasItem`
- concurrent `MarkItemSeen`
- concurrent source-initialization checks
- concurrent attempts to establish one source baseline

must not race.

The atomic baseline operation selects one winning initializer, but it does not
define collection scheduling order. Application orchestration should serialize
collection runs for the same source if the earliest successful collection must
strictly win.

Run:

```bash
go test -race ./...
```

if practical.

---

## Non-Goals

Do not implement:

- SQLite
- PostgreSQL
- Redis
- file-backed state
- persistence migrations
- UPDATE detection
- DELETE detection
- item versioning
- content snapshots
- NLP summary storage
- notification state
- notification retry
- outbox
- scheduler
- Telegram
- NLP
- external adapter protocols

---

## Suggested Package Direction

Adapt to the repository's existing layout.

A reasonable shape is:

```text
internal/
├── state/
│   ├── store.go
│   ├── memory.go
│   └── memory_test.go
│
└── itemkey/
    ├── builder.go
    └── builder_test.go
```

Do not restructure unrelated packages solely to match this suggestion.

---

## Required Tests

### Item key tests

Cover at minimum:

- ExternalID wins over all lower-priority candidates
- URL used when ExternalID is absent
- PublishedAt + Title used when ExternalID and URL are absent
- Title + Content hash used as final deterministic fallback
- same input produces same key
- different sources remain isolated in store scope
- error when no deterministic identity fallback is possible

### Memory store tests

Cover at minimum:

- new source is not initialized
- initialized source becomes initialized
- unseen item returns false
- marked item returns true
- source isolation
- item-key isolation
- concurrent reads/writes do not corrupt state

### Baseline behavior tests

- first successful collection marks all current items seen
- first successful empty collection still initializes source
- baseline items are not returned as unseen work
- subsequent new item is treated as unseen
- collection failure does not initialize source
- one invalid item leaves baseline state unchanged
- concurrent baseline establishment has only one winner

---

## Acceptance Criteria

- [ ] deterministic item-key builder exists
- [ ] `ExternalID -> URL -> PublishedAt+Title -> content hash` fallback is implemented
- [ ] `StateStore` abstraction exists
- [ ] `MemoryStateStore` exists
- [ ] `MemoryStateStore` is concurrency-safe
- [ ] source initialization is tracked independently from seen items
- [ ] baseline keys and source initialization are committed atomically
- [ ] first successful empty collection can complete baseline
- [ ] baseline items can be marked seen without downstream processing
- [ ] unseen items are not automatically marked seen on discovery
- [ ] no durable database/cache is introduced
- [ ] no UPDATE/DELETE logic is introduced
- [ ] unit tests pass
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` is run if practical
- [ ] architecture/status docs are updated if implementation changes documented decisions

---

## Subagent Instructions

Before changing code:

1. inspect the repository,
2. read `AGENTS.md`,
3. read the current project and architecture documentation,
4. read relevant source architecture documents,
5. inspect the current `MonitorItem` and package conventions.

Do not duplicate abstractions already present in the repository.

Do not change unrelated Source or Telegram implementations.

Do not implement NLP.

Do not implement durable persistence.

When finished, report:

1. files created/modified,
2. interfaces/types introduced,
3. item-key behavior,
4. baseline behavior,
5. concurrency approach,
6. tests executed and results,
7. assumptions/deviations,
8. unresolved questions for architecture discussion.
