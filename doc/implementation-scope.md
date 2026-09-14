# Initial Implementation Scope

## Goal

Produce the first working source-side skeleton of Auto Alert without prematurely implementing downstream alert semantics.

## Deliverables

The first implementation should include:

1. Python project/package setup
2. `SourceDefinitionDocument`
3. `SourceDefinitionRepository` protocol
4. `FileSourceDefinitionRepository`
5. `ConfigDecoder` protocol
6. `YamlConfigDecoder`
7. source config schema version 1 parsing/validation
8. generic `SourceConfig`
9. HTML-specific typed settings
10. `SourceAdapter` protocol
11. `SourceAdapterRegistry`
12. `SourceRunner`
13. `HtmlSourceAdapter`
14. HTTP GET abstraction
15. HTML extraction using CSS selectors
16. smallest practical provisional `MonitorItem`
17. unit tests using local fixtures
18. one example source YAML
19. basic README instructions for running tests and demonstrating config loading

## Required Architectural Behavior

The implementation must demonstrate that these concerns are independent:

```text
Storage:
FileSourceDefinitionRepository
        |
        v
SourceDefinitionDocument

Encoding:
YamlConfigDecoder
        |
        v
decoded config

Schema:
version 1 validation
        |
        v
SourceConfig

Runtime:
SourceAdapterRegistry
        |
        v
HtmlSourceAdapter
        |
        v
MonitorItem[]
```

## Non-Goals

Do not implement:

- Telegram
- alert rules
- scheduler
- SQLite
- change detector
- item history
- notification outbox
- Kafka
- Redis
- Celery
- Kubernetes client
- database-backed source repository
- RSS
- JSON source adapter
- Playwright
- web UI

Interfaces should make those future additions possible where already discussed, but there is no requirement to create empty implementations for them.

## Acceptance Criteria

The implementation is acceptable when all of the following are true:

1. A YAML file can be read through `FileSourceDefinitionRepository`.
2. YAML decoding is independent from file reading.
3. Schema version 1 is required and validated.
4. A validated config becomes a generic `SourceConfig`.
5. `SourceRunner` resolves the `html` adapter through a registry.
6. The HTML adapter can process fixture HTML into provisional `MonitorItem` objects.
7. Relative links are converted to absolute URLs.
8. Parser unit tests do not require network access.
9. Unsupported source types fail with a clear error.
10. No downstream change-detection semantics are silently introduced.
11. All tests pass.

## Implementation Bias

When multiple implementations satisfy the architecture:

- choose the simpler one,
- avoid framework-heavy solutions,
- avoid dependency injection containers,
- avoid unnecessary base classes,
- prefer protocols/composition,
- keep public contracts small.

## Expected Subagent Report

After implementation, report:

- files created or changed,
- dependency choices,
- final public interfaces,
- any deviations from the architecture,
- assumptions made,
- tests executed and results,
- unresolved questions that should return to architecture discussion.
