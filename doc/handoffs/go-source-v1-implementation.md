# Handoff: Go Source v1 Implementation

Implement the current SignalWatch source-side architecture in Go.

Before making changes, read:

- `AGENTS.md`
- `doc/architecture.md`
- `doc/source-architecture.md`
- `doc/language-strategy.md`
- `doc/implementation-scope.md`

These documents are authoritative for this iteration.

## Goal

Produce a clean, tested Go implementation of:

```text
File source definitions
        |
        v
YAML decode
        |
        v
schema v1 validation
        |
        v
SourceConfig
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
        v
MonitorItem[]
```

## Important Architecture Constraint

SignalWatch Core is Go.

The architecture permits future adapters in other languages, but **do not implement cross-language support now**.

Do not create:

- Python code,
- Java code,
- subprocess worker protocol,
- HTTP adapter service,
- gRPC,
- protobuf,
- JSON Schema.

The only requirement is that the Go core does not make future external adapters impossible.

## Implementation Tasks

1. Inspect the current repository before creating files.
2. Reuse or adjust existing package structure where reasonable.
3. Ensure the repository is a valid Go module.
4. Implement the source-definition repository boundary.
5. Implement file-backed definitions.
6. Implement YAML decoding separately from file access.
7. Implement schema v1 parsing/validation.
8. Implement generic `SourceConfig`.
9. Implement HTML-specific typed settings.
10. Implement `SourceAdapter`.
11. Implement adapter registry.
12. Implement `SourceRunner`.
13. Implement native Go HTML collection.
14. Keep HTTP fetching and HTML extraction independently testable.
15. Use fixture HTML and table-driven tests.
16. Use `httptest.Server` rather than live websites.
17. Add one example source definition.
18. Run `go test ./...`.

## Dependency Guidance

Use the standard library where practical.

A focused YAML package and a focused HTML/CSS selector library are acceptable.

Do not add a general web framework, DI container, scheduler framework, message broker, or ORM.

## Unresolved Domain Areas

Do not permanently define:

- item identity,
- content hashes,
- canonical URL policy beyond basic relative URL resolution,
- NEW/UPDATED semantics,
- baseline behavior,
- deletion behavior,
- notification semantics.

Use the smallest reversible provisional `MonitorItem`.

## Completion Report

Return:

1. files created/modified,
2. public/internal interfaces introduced,
3. dependencies added and why,
4. tests executed and results,
5. assumptions made,
6. deviations from docs, if any,
7. unresolved questions that should return to architecture discussion.
