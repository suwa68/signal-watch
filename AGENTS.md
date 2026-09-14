# AGENTS.md

## Project Overview

This repository contains **Auto Alert**, a configurable source-monitoring and notification system.

The long-term goal is to monitor external information sources, detect meaningful changes, evaluate rules, and deliver notifications to configured destinations such as Telegram.

The project is intentionally starting small, but architectural boundaries must preserve future extensibility.

## Current Implementation Scope

The current implementation scope is limited to the parts already agreed upon:

1. Source-definition loading
2. Source configuration decoding and validation
3. Source adapter abstraction
4. HTML source adapter v1
5. Runtime orchestration from `SourceConfig` to `MonitorItem[]`

Do **not** design or implement the following beyond the minimum placeholders needed for compilation/tests:

- Change detection semantics
- Baseline / first-run semantics
- Rule engine behavior
- Notification delivery semantics
- Transactional outbox
- Database persistence model
- Deletion detection
- LLM classification
- Distributed workers
- Redis / Kafka / RabbitMQ / Celery
- Kubernetes-specific runtime logic

These areas are intentionally left for later architecture discussions.

## Architecture Principles

### 1. Simple implementation, stable boundaries

Version 1 should be easy to understand and operate, but should not tightly couple implementation details to the rest of the system.

### 2. Normalize at the edge

All source types must eventually produce a common runtime representation:

`SourceAdapter -> MonitorItem[]`

The monitoring pipeline must not depend on HTML, RSS, JSON, Kubernetes, or any source-specific storage mechanism.

### 3. Extend by adding implementations, not rewriting the pipeline

Future source types should be added by implementing new adapters.

Future source-definition storage backends should be added by implementing new repositories.

### 4. Infrastructure concerns must not leak into the domain

File paths, Kubernetes ConfigMaps, database row metadata, S3 ETags, YAML syntax, HTTP libraries, and BeautifulSoup objects must not leak into core runtime models.

### 5. Avoid premature abstraction

Only create abstractions that correspond to concrete extension points already identified in the architecture.

Do not build a plugin framework, DSL, generic workflow engine, or distributed system.

## Core Contracts

The following boundaries are considered important and should be preserved.

### Source Definition Storage

`SourceDefinitionRepository`

Answers:

> Where do source definitions come from?

Version 1 implementation:

`FileSourceDefinitionRepository`

Potential future implementations:

- `DatabaseSourceDefinitionRepository`
- `KubernetesSourceDefinitionRepository`
- `S3SourceDefinitionRepository`

Kubernetes ConfigMaps mounted as files should normally continue to use `FileSourceDefinitionRepository`.

### Source Definition Document

A repository returns a storage-neutral document.

Conceptually:

```python
@dataclass(frozen=True)
class SourceDefinitionDocument:
    key: str
    revision: str | None
    content: str
```

`revision` describes the revision of the specific definition instance, not the config schema version.

Examples of possible future revision sources:

- file content hash or mtime
- database version column
- Kubernetes `resourceVersion`
- S3 ETag

### Configuration Decoding

`ConfigDecoder`

Answers:

> How is the raw source definition encoded?

Version 1:

`YamlConfigDecoder`

Potential future implementation:

`JsonConfigDecoder`

A repository must not be responsible for understanding YAML semantics.

### Config Schema Version

Every source definition must include a schema version.

Example:

```yaml
version: 1
```

Schema version means:

> Which structure/semantics does this source definition use?

It is distinct from document `revision`.

### Runtime Source Configuration

Decoded and validated configuration becomes `SourceConfig`.

`SourceConfig` must not expose persistence-specific metadata.

It should contain generic top-level source information and adapter-specific configuration.

### Source Adapter

`SourceAdapter`

Answers:

> How is data collected from this source?

Conceptual contract:

```python
class SourceAdapter(Protocol):
    async def collect(
        self,
        source: SourceConfig,
    ) -> list[MonitorItem]:
        ...
```

Version 1 implementation:

`HtmlSourceAdapter`

Potential future implementations:

- `RssSourceAdapter`
- `JsonApiSourceAdapter`
- `BrowserSourceAdapter`
- custom domain-specific adapters

### Adapter Registry

Avoid large `if/elif` chains for adapter selection.

Use a small registry abstraction that resolves:

`source.type -> SourceAdapter`

Version 1 only needs the `html` registration.

## Source v1 Scope

Source v1 supports:

- public HTTP/HTTPS URLs
- HTTP GET
- static or server-rendered HTML
- repeated item elements
- CSS selector extraction
- title extraction
- URL extraction
- content extraction
- relative URL resolution
- YAML source definitions

Source v1 does not support:

- JavaScript rendering
- authentication
- cookies/session login
- pagination
- POST APIs
- RSS
- JSON APIs
- CAPTCHA handling
- anti-bot bypass
- arbitrary transformation DSLs

## Source v1 Configuration

Recommended shape:

```yaml
version: 1

id: murata_news
name: Murata News
type: html

interval_seconds: 300

config:
  url: https://example.com/news

  selectors:
    item: ".news-item"
    title: ".title"
    url: "a"
    content: ".summary"
```

Keep adapter-specific fields under `config`.

The generic `SourceConfig` must not be permanently shaped around HTML selectors.

## Technology Direction

For the current implementation:

- Python 3.12+
- asyncio
- httpx
- BeautifulSoup
- PyYAML
- pytest

A scheduler may be introduced later, but the first source-runtime implementation should be independently testable without scheduling.

## Testing Expectations

Core behavior must be testable without network access.

At minimum, add tests for:

- loading source definitions from files
- YAML decoding
- schema version validation
- invalid source definitions
- adapter registry lookup
- HTML parsing into `MonitorItem`
- relative URL resolution
- missing optional fields
- unsupported adapter types

Use fixture HTML rather than live external websites.

## Security

Never commit:

- Telegram bot tokens
- API keys
- passwords
- private credentials

Do not introduce secrets into source-definition examples unless they are placeholders.

## Coding Expectations

Prefer:

- explicit types
- small focused modules
- dependency injection for I/O boundaries
- deterministic parsing logic
- immutable models when practical
- clear errors with source-definition identity/context

Avoid:

- global mutable registries
- source-specific behavior inside generic orchestration
- direct YAML parsing inside runtime source adapters
- direct file access inside config validation
- premature generic frameworks

## Before Finishing a Change

1. Run relevant tests.
2. Keep architecture documents synchronized with intentional architectural changes.
3. Do not silently decide unresolved domain semantics.
4. Summarize files changed, tests run, and any assumptions made.
