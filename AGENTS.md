# AGENTS.md

## Project

SignalWatch is a configurable source-monitoring and notification engine.

## Architecture Invariants

- Start as a modular monolith; avoid premature microservices.
- Implement the core runtime in Go and use Go for source adapters by default.
  Specialized adapters may use another language when a concrete requirement
  justifies it, while honoring the same logical adapter contract.
- Normalize source-specific data at the edge:
  `SourceConfig -> SourceAdapter -> MonitorItem[]`. The core and `SourceRunner`
  must remain independent of adapter implementation language and transport.
- Extend source types through adapters and definition storage through repositories.
  Keep storage, decoding/schema validation, and collection responsibilities separate.
  Mounted files, including ConfigMaps, normally use the file repository.
- Keep infrastructure details (serialization, filesystem paths, database metadata,
  transports, and SDK types) out of core models.
- Prefer the standard library and focused dependencies over speculative infrastructure.

## Development Workflow

1. Before implementation, find an existing GitHub issue covering the feature;
   reuse it or create one with the problem, scope, non-goals, and acceptance criteria.
2. Create and switch to a dedicated `feat/<feature-name>` branch from `main`
   unless the task specifies another base. Do not commit feature work to `main`.
3. Keep implementation, tests, and related documentation on that branch.
4. Reference the issue with `Closes #<number>` in the PR and include validation results.
5. Report the issue link and branch name when handing off the work.

## Execution Authority

Once execution scope is approved, the Parent chooses the execution method and
number of implementation subagents unless the user specifies them. Preserve all
applicable Superpowers skill requirements, including concurrency restrictions
and independent reviews. Do not impose a fixed subagent count.

Ordinary status questions do not suspend already authorized work unless the user
explicitly requests a pause.

## Scope & Deferrals

Existing v1 behavior is a baseline, not a permanent restriction on explicitly
approved future development. Historical phase labels are not current scope approval.

Deferred capabilities require explicit scope approval. Approval lifts only the
specific deferral being approved; all other deferrals remain in force.
Do not silently finalize unresolved domain semantics.

The following capabilities remain outside the approved
implementation scope. Do not make permanent design
decisions or implement them without specific approval.
Exploratory analysis does not constitute implementation approval.

- External adapter runtimes/processes or cross-language transports, including gRPC,
  HTTP adapter services, subprocess protocols, JSON Schema for `MonitorItem`, or protobuf.
- Final item identity beyond provisional v1 keys, content-diff hashing or snapshots,
  change detection beyond seen/unseen, deletion semantics, or durable/cross-process baselines.
- Scheduling, persistence, durable retry, notification outbox, NLP, rule engine,
  or destination routing.
- Distributed workers, message brokers, Redis, Kafka, RabbitMQ, Kubernetes runtime
  integration, web frameworks, or DI containers.

Source v1 covers public HTTP/HTTPS GET of static/server-rendered HTML. Browser
rendering, authentication, pagination, POST APIs, RSS, JSON API adapters,
CAPTCHA/anti-bot bypass, and custom transformation DSLs remain outside v1 unless
specifically approved.

## Validation & Security

- Run relevant tests before completing work and report actual validation results.
- Tests must not depend on live external websites; use local fixtures and
  `httptest.Server` where HTTP behavior is needed.
- Keep affected architecture and feature documentation synchronized with intentional changes.
- Report assumptions and deviations.
- Never commit Telegram bot tokens, API keys, credentials, or secrets.

## Reference Documents

- [README](README.md): implemented capabilities, entry points, and development commands.
- [Architecture](doc/architecture.md) and [language strategy](doc/language-strategy.md):
  system boundaries and language direction.
- [Source architecture](doc/source-architecture.md): repository, decoder, schema,
  registry, adapter, and source v1 contracts. The [initial implementation scope](doc/implementation-scope.md)
  records only the historical source-side iteration, not every implemented v1 feature.
- [Memory state and baseline](doc/handoffs/memory-state-baseline-v1.md): item keys,
  atomic baseline, seen state, and restart limitations.
- [Notification orchestration](doc/notification-orchestration-v1.md) and
  [Telegram v1](doc/telegram-v1.md): delivery, assembly, cancellation, and rendering semantics.
- [Query v1 handoff](doc/handoffs/signalwatch-ai-query-v1-handoff.md): CLI, MCP,
  and JSON contracts; its dated baseline is historical. See README for implemented status.
- [HTML resource safety](doc/html-resource-safety.md) and
  [real source verification](doc/real-source-verification.md): limits and fixture/live evidence boundaries.
- [CI validation](doc/ci-reliability.md) and [Docker MCP Tunnel](doc/docker-mcp-tunnel.md):
  validation commands and packaging guidance.
