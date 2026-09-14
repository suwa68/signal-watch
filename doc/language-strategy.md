# Language Strategy

## Decision

SignalWatch Core will be implemented in **Go**.

Source adapters are implemented in Go by default, but the architecture permits specialized adapters in other languages when a concrete requirement justifies it.

## Why Go for the Core

SignalWatch behaves primarily like a long-running monitoring daemon:

```text
many sources
    |
    v
network I/O
    |
    v
concurrent collection
    |
    v
state processing
    |
    v
notification delivery
```

Go is a good fit for:

- long-running services,
- I/O concurrency,
- explicit cancellation,
- lightweight deployment,
- simple containerization,
- stable interfaces,
- operational tooling.

## Why Not Require Go Everywhere

Some future source types may depend on ecosystems where another language is materially better.

Examples:

### Python

Useful for:

- browser automation,
- Playwright-heavy extraction,
- AI/NLP workflows,
- data-processing libraries.

### Java

Potentially useful for:

- enterprise integrations,
- JVM-only SDKs,
- organization-specific infrastructure.

The core architecture should not make these integrations impossible.

## Default Rule

Use Go unless another language offers a concrete advantage for a specific adapter.

Do not introduce polyglot complexity speculatively.

## Evolution Strategy

### Phase 1

Everything runs in Go.

### Phase 2

If a specialized adapter cannot reasonably be implemented in Go, introduce an `ExternalSourceAdapter`.

Prefer the simplest process boundary first.

A likely candidate is:

```text
stdin/stdout + JSON
```

This is not yet a committed protocol.

### Phase 3

Only if independent scaling, isolation, or deployment becomes necessary, consider a remote adapter protocol.

Potential options:

- HTTP
- gRPC

## Non-Decision

The following are explicitly not selected yet:

- subprocess wire format,
- external adapter lifecycle,
- JSON Schema,
- protobuf,
- HTTP API shape,
- gRPC service definition,
- worker discovery,
- remote authentication.

These choices depend on the final source and `MonitorItem` contracts.
