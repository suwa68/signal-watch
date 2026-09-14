# Implementation Prompt for Subagent

Implement the first source-side iteration of **Auto Alert**.

Before making changes, read:

- `AGENTS.md`
- `doc/architecture.md`
- `doc/source-architecture.md`
- `doc/implementation-scope.md`

Treat those files as authoritative for the current iteration.

## Important constraints

Do not attempt to implement the full product.

This iteration is intentionally limited to:

- source-definition storage abstraction,
- YAML decoding,
- schema version 1,
- generic source configuration,
- source adapter abstraction,
- HTML source adapter v1,
- conversion into a provisional `MonitorItem`.

Do not make permanent design decisions for:

- `MonitorItem` identity,
- change detection,
- first-run/baseline semantics,
- rule evaluation,
- persistence,
- notification delivery.

If implementation requires a provisional choice in an unresolved area, choose the smallest reversible option and document it.

## Work requested

1. Inspect the repository.
2. Create or adjust the Python package structure.
3. Add only the dependencies required for the current scope.
4. Implement the contracts and v1 implementations described in the docs.
5. Add unit tests and fixture HTML.
6. Add one example YAML source definition.
7. Run the tests.
8. Report:
   - files changed,
   - interfaces introduced,
   - tests run/results,
   - assumptions,
   - unresolved questions.

Prefer simple composition and Python protocols over inheritance-heavy frameworks.
