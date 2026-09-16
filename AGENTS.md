# AGENTS.md

## Project

This repository contains **SignalWatch**.

SignalWatch is a configurable source-monitoring and notification engine. It is intended to run continuously, collect data from external sources, normalize source-specific data into stable internal contracts, detect meaningful changes, evaluate rules, and deliver notifications.

The project should start as a modular monolith.

## Feature Development Workflow

Every feature must have a GitHub issue and be developed on its own branch.

1. Before implementation, check for an existing issue covering the feature. Reuse
   it when appropriate; otherwise create an issue describing the problem, scope,
   non-goals, and acceptance criteria.
2. Create and switch to a dedicated branch before editing feature code. Follow
   the existing `feat/<feature-name>` convention and branch from `main` unless
   the task explicitly specifies another base.
3. Keep the feature's implementation, tests, and related documentation on that
   branch. Do not commit feature work directly to `main`.
4. When opening a pull request, reference the issue with `Closes #<number>` and
   include the relevant validation results.
5. Report the issue link and branch name when handing off the work.

## Current Language Direction

The **SignalWatch Core is implemented in Go**.

Go is responsible for the long-running monitoring runtime and application orchestration, including:

- configuration loading,
- source runtime orchestration,
- scheduling,
- concurrency control,
- persistence,
- change detection,
- rule evaluation,
- notification routing,
- Telegram delivery,
- observability,
- application lifecycle.

Source adapters should be implemented in Go by default.

However, the architecture must preserve the ability to implement specialized adapters in other languages when justified by ecosystem or tooling needs.

Examples:

- Python for Playwright-heavy browser automation,
- Python for AI/NLP/data-processing integrations,
- Java for domain-specific enterprise integrations,
- any other runtime that can honor the Source Adapter contract.

Do not force every future adapter to be written in Go.

## Core Architecture Principle

The most important source-side contract is:

```text
SourceConfig
    |
    v
Source Adapter
    |
    v
MonitorItem[]
```

The core runtime must not depend on how the adapter is implemented.

A Go-native adapter and a future external adapter must be interchangeable from the perspective of `SourceRunner`.

## Current Scope

The current implementation scope is:

1. Go project bootstrap
2. source-definition repository abstraction
3. file-based source-definition repository
4. YAML decoding
5. source config schema version 1
6. source adapter abstraction
7. adapter registry
8. source runner
9. native Go HTML source adapter
10. provisional `MonitorItem`
11. tests using local fixtures
12. transport-independent `Notification` v1 model
13. outbound Telegram destination, HTML renderer, and single-attempt notifier
14. explicit Telegram smoke command and local SDK integration tests
15. deterministic v1 item keys
16. concurrency-safe in-memory seen-item state
17. atomic first-success baseline handling

Telegram v1 is specified in `doc/telegram-v1.md`. The notification model is a
completed delivery input; deriving it from source items is not implemented yet.

The first implementation must **not** introduce a second runtime or external adapter process yet.

Polyglot support is an architectural extension point, not a v1 deployment requirement.

## Do Not Implement Yet

Do not make permanent decisions or large implementations for:

- cross-language transport
- gRPC
- HTTP adapter services
- subprocess adapter protocol
- JSON Schema for `MonitorItem`
- protobuf schemas
- final item identity beyond the provisional v1 key strategy
- content-diff hashing or snapshot semantics
- change detection semantics beyond seen vs unseen
- durable or cross-process baseline behavior
- deletion semantics
- rule engine
- persistence
- notification outbox
- distributed workers
- Redis
- Kafka
- RabbitMQ
- Kubernetes runtime integration

These topics remain intentionally open.

## Architecture Principles

### Simple implementation, stable boundaries

Keep v1 operationally simple while preserving stable extension points.

### Go by default, polyglot when justified

Do not introduce another language merely because it may be useful later.

Use another runtime only when a concrete adapter requirement justifies it.

### Normalize at the edge

Source-specific details must be converted into `MonitorItem` before entering downstream monitoring logic.

### Extend by adding implementations

New source types should be added as adapters.

New source-definition storage backends should be added as repositories.

Avoid rewriting the core pipeline.

### Infrastructure details must not leak into core models

The core must not depend on:

- YAML syntax,
- filesystem paths,
- Kubernetes ConfigMaps,
- BeautifulSoup,
- Playwright,
- database row metadata,
- transport-specific request types.

### Avoid premature microservices

A possible future Python adapter does not justify creating a Python service today.

## Core Contracts

### SourceDefinitionRepository

Answers:

> Where do source definitions come from?

Go-facing contract should be equivalent to:

```go
type SourceDefinitionRepository interface {
    List(ctx context.Context) ([]SourceDefinitionDocument, error)
}
```

Version 1:

`FileSourceDefinitionRepository`

Future examples:

- database repository
- S3 repository
- Kubernetes-aware repository

A ConfigMap mounted as files should normally still use the file repository.

### SourceDefinitionDocument

Conceptually:

```go
type SourceDefinitionDocument struct {
    Key      string
    Content  []byte
    Revision string
    Origin   string
}
```

The exact Go representation may be adjusted if needed.

`Revision` is distinct from config schema version.

### Config decoding

The repository does not parse YAML.

A decoder handles serialization.

Version 1:

`YAMLConfigDecoder`

### Source config schema version

Every definition includes:

```yaml
version: 1
```

Unknown versions must fail clearly.

### SourceAdapter

Go-facing runtime contract:

```go
type SourceAdapter interface {
    Collect(
        ctx context.Context,
        source SourceConfig,
    ) ([]MonitorItem, error)
}
```

Version 1 implementation:

`HTMLSourceAdapter`

Future implementations may be native Go adapters or external adapters.

### SourceAdapterRegistry

Resolve:

```text
source.type -> SourceAdapter
```

Avoid large `switch`/`if` chains in runtime orchestration.

### SourceRunner

`SourceRunner` resolves the adapter and executes collection.

It must not know whether an adapter internally uses:

- net/http,
- goquery,
- another Go library,
- a subprocess,
- gRPC,
- HTTP,
- Python,
- Java.

## Source v1 Scope

Source v1 supports:

- public HTTP/HTTPS
- GET requests
- static/server-rendered HTML
- repeated item elements
- CSS selector extraction
- title extraction
- optional URL extraction
- optional content extraction
- relative URL resolution
- YAML source definitions

Source v1 does not support:

- browser rendering
- authentication
- pagination
- POST APIs
- RSS
- JSON API adapters
- CAPTCHA
- anti-bot bypass
- custom transformation DSL

## Go Implementation Guidance

Prefer standard library packages unless a focused library materially improves clarity.

Expected baseline:

- Go 1.25+ if repository/toolchain permits; otherwise use the repository's chosen supported Go version
- `context`
- `net/http`
- YAML library
- focused HTML parsing library
- Go standard testing package

Do not introduce a web framework, DI container, or message broker for this phase.

## Testing

Tests must not depend on live external websites.

Use:

- fixture YAML
- fixture HTML
- `httptest.Server` where HTTP behavior is needed

At minimum test:

- file repository behavior
- YAML decoding
- schema version validation
- invalid definitions
- registry lookup
- unsupported source types
- HTML extraction
- optional fields
- relative URL resolution
- cancellation propagation where practical

## Security

Never commit:

- Telegram bot tokens
- API keys
- credentials
- secrets

## Before Completing Work

1. Run relevant tests.
2. Keep architecture docs synchronized with intentional architecture changes.
3. Do not silently finalize unresolved domain semantics.
4. Report assumptions and deviations.
