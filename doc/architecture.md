# SignalWatch Architecture

## 1. Purpose

SignalWatch is a long-running information-monitoring engine.

Its eventual responsibility is:

```text
Source Definition
        |
        v
Source Runtime
        |
        v
MonitorItem[]
        |
        v
Change Detection
        |
        v
Rule Engine
        |
        v
Notification Routing
        |
        v
Delivery Adapters
```

The source-definition and source-runtime portions and an isolated outbound
Telegram destination are currently implemented. The stages connecting source
items to completed notifications remain future work.

## 2. Architectural Style

SignalWatch begins as a **modular monolith**.

Initial characteristics:

- one repository,
- one primary Go application,
- one deployable runtime,
- no distributed queue,
- no service mesh,
- no microservice decomposition.

Clear internal boundaries should allow later decomposition if actual scale or operational requirements justify it.

## 3. Language Strategy

### 3.1 SignalWatch Core

The SignalWatch Core is implemented in **Go**.

Go owns the long-running runtime responsibilities:

```text
Config
Scheduling
Concurrency
Source Runtime
Change Detection
Rules
Persistence
Notification Routing
Telegram
Observability
Lifecycle
```

### 3.2 Adapter language policy

Source adapters are implemented in Go by default.

However, adapter implementation language is not part of the core domain contract.

Future specialized adapters may use another language when justified.

Examples:

```text
Go native HTML adapter
Go native RSS adapter
Go native JSON API adapter

Python browser adapter
Python AI/NLP adapter
Java enterprise adapter
```

The language boundary must remain outside downstream monitoring logic.

## 4. Stable Runtime Boundary

The fundamental source-side runtime contract is:

```text
SourceConfig
    |
    v
SourceAdapter
    |
    v
MonitorItem[]
```

Downstream code should never need to know:

- whether parsing occurred in-process,
- which parser library was used,
- whether an adapter used Go or another language,
- whether a future external adapter uses subprocess, HTTP, or gRPC.

## 5. Native and External Adapters

The architecture anticipates two implementation classes:

```text
SourceAdapter
    |
    +-- Native Adapter
    |
    +-- External Adapter
```

### Native Adapter

Runs in the SignalWatch Go process.

Version 1 uses this model.

Examples:

- HTML
- RSS
- JSON API

### External Adapter

Runs outside the Go process.

Potential future implementations:

- subprocess adapter,
- local worker,
- remote HTTP adapter,
- gRPC adapter.

No external adapter transport is selected yet.

## 6. Important Constraint

Polyglot capability is an **extension point**, not a requirement to create multiple services today.

Version 1 must not introduce:

- Python sidecars,
- gRPC servers,
- adapter daemons,
- service discovery,
- message brokers.

The first implementation is fully Go.

## 7. Source Definition Architecture

Source configuration is separated into three concerns.

### Storage

```text
SourceDefinitionRepository
```

Answers:

> Where do source definitions come from?

Version 1:

```text
FileSourceDefinitionRepository
```

Potential future sources:

- DB
- S3
- Kubernetes API
- remote config service

### Serialization

```text
ConfigDecoder
```

Answers:

> How is the definition encoded?

Version 1:

```text
YAML
```

Future:

```text
JSON
```

### Semantics

```text
Schema Version
        |
        v
Validation
        |
        v
SourceConfig
```

Answers:

> What does this definition mean?

## 8. Schema Version vs Definition Revision

These are separate concepts.

### Schema version

```yaml
version: 1
```

Means:

> Which config schema and semantics are used?

### Definition revision

Means:

> Which revision of this specific source definition is currently loaded?

Possible future revision sources:

- file hash,
- mtime,
- database version,
- Kubernetes resourceVersion,
- S3 ETag.

## 9. Version 1 Source Runtime

```text
File Source Definitions
        |
        v
YAML Decoder
        |
        v
Schema v1
        |
        v
SourceConfig(type=html)
        |
        v
SourceRunner
        |
        v
Adapter Registry
        |
        v
HTMLSourceAdapter
        |
        +--> HTTP GET
        |
        +--> HTML extraction
        |
        v
MonitorItem[]
```

## 10. External Adapter Direction

When a real requirement appears for a non-Go adapter, the preferred evolution path is:

### Stage 1

Native Go adapter.

### Stage 2

External process adapter if cross-language support is needed but service separation is not.

Conceptual model:

```text
SignalWatch Go
     |
     +--> launch external process
     |
     +--> request on stdin
     |
     <-- response on stdout
```

The likely payload format would be JSON, but this is not yet a committed protocol.

### Stage 3

Only when scale or operational isolation requires it:

```text
SignalWatch Core
     |
     +--> remote adapter service
```

Possible transports may include:

- HTTP
- gRPC

No transport is selected today.

## 11. Cross-Language Contract Direction

If/when external adapters are introduced, the contract must be language-neutral.

Potential future representations:

- JSON Schema
- protobuf

However, this must wait until the `MonitorItem` domain model is intentionally designed.

Do not freeze a cross-language schema based on the provisional v1 model.

## 12. Notification and Telegram v1

The first delivery contract is independent of source collection and SDK types:

```text
notification.Notification
        |
        v
telegram.Renderer (escaped HTML and summary length handling)
        |
        v
telegram.Notifier (one SDK SendMessage call)
        |
        v
telegram.Destination (ID and string ChatID)
```

`Notification` contains required plain-text `Title`, `Summary`, and `SourceName`,
plus optional `URL` and `PublishedAt`. It contains no Telegram markup or NLP
metadata. The mapping from `MonitorItem` to this completed delivery input is
not yet designed.

Application assembly provides one secret bot token to `telegram.New` and reuses
the notifier for multiple destinations. The Telegram SDK stays inside
`internal/notification/telegram`. Initialization uses `getMe`; sending does not
start polling, webhooks, or handlers. No retry or delivery persistence is added.

See [Telegram v1](telegram-v1.md) for the concrete API, rendering policies, and
manual smoke test.

## 13. Explicitly Unresolved Areas

The following remain intentionally unresolved:

- final `MonitorItem` schema,
- item identity,
- canonical URL rules,
- content hashing,
- change detection,
- baseline behavior,
- deletion detection,
- scheduling details,
- retry/backoff,
- persistence,
- producing `Notification` from source items (including NLP and rule evaluation),
- destination routing and global notification configuration,
- delivery reliability,
- external adapter transport,
- protobuf / JSON Schema.
