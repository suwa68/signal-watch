# Initial Go Implementation Scope

## Objective

Create the first Go implementation of the SignalWatch source-side architecture.

The implementation should prove the architectural boundaries without building the full product.

## Required Deliverables

1. Initialize/verify the Go module.
2. Create source-definition models and repository interfaces.
3. Implement `FileSourceDefinitionRepository`.
4. Create config decoder abstraction.
5. Implement YAML decoder.
6. Enforce config schema `version: 1`.
7. Create generic `SourceConfig`.
8. Create typed HTML adapter settings.
9. Define `SourceAdapter`.
10. Implement `SourceAdapterRegistry`.
11. Implement `SourceRunner`.
12. Implement native Go `HTMLSourceAdapter`.
13. Implement HTTP GET boundary.
14. Implement CSS-selector-based HTML extraction.
15. Add provisional `MonitorItem`.
16. Add fixture-based tests.
17. Add one example YAML source definition.
18. Add minimal developer documentation needed to run tests.

## Required Behavior

The implementation must demonstrate separation between:

```text
Storage
FileSourceDefinitionRepository

Serialization
YAMLConfigDecoder

Schema
Config v1 validation

Runtime
SourceRunner
Adapter Registry
HTMLSourceAdapter

Output
MonitorItem[]
```

## Language Requirement

The current implementation is Go-only.

Do not create:

- Python code,
- Java code,
- external worker services,
- subprocess adapters,
- gRPC definitions.

The architecture should leave room for those later.

## Non-Goals

Do not implement:

- scheduler,
- Telegram,
- rule engine,
- change detector,
- persistence,
- outbox,
- RSS,
- JSON API source,
- Playwright,
- database source repository,
- Kubernetes client,
- external adapter transport,
- final cross-language schema.

## Acceptance Criteria

1. `go test ./...` passes.
2. File repository and YAML decoding are separate.
3. Schema version 1 is mandatory.
4. Generic config is not HTML-specific.
5. SourceRunner resolves adapter through registry.
6. HTML adapter can parse fixture HTML.
7. Relative links are resolved correctly.
8. Network-dependent tests use `httptest.Server` or mocks/fakes.
9. Unsupported source types return clear errors.
10. No unresolved downstream semantics are silently finalized.
11. No non-Go runtime is introduced.

## Implementation Bias

Prefer:

- standard library,
- small focused interfaces,
- explicit constructors,
- table-driven tests,
- composition,
- `context.Context`,
- minimal dependencies.

Avoid:

- DI frameworks,
- web frameworks,
- global mutable registries,
- premature generic frameworks,
- reflection-heavy designs.
