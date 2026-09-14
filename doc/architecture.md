# Auto Alert Architecture

## 1. Purpose

Auto Alert is intended to become a reusable information-monitoring platform.

The system will eventually:

1. load configured sources,
2. collect data from those sources,
3. convert source-specific data into common domain objects,
4. detect meaningful changes,
5. evaluate alert rules,
6. deliver notifications.

The project starts with a narrow source implementation while preserving clean extension points.

## 2. Architectural Style

The project should begin as a **modular monolith**.

Initial deployment characteristics:

- one repository
- one application
- one process unless operational needs later justify otherwise
- no distributed messaging infrastructure
- clear internal module boundaries

Do not start with microservices.

## 3. High-Level Direction

The long-term pipeline is expected to resemble:

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
ChangeEvent[]
        |
        v
Rule Engine
        |
        v
Alert
        |
        v
Notification Delivery
```

Only the **Source Definition** and **Source Runtime** sections are currently specified in sufficient detail for implementation.

The remaining sections are intentionally unresolved.

## 4. Source Architecture Layers

Source configuration is separated into three architectural concerns.

### 4.1 Storage abstraction

Answers:

> Where does the source definition come from?

Contract:

`SourceDefinitionRepository`

Version 1:

`FileSourceDefinitionRepository`

Future examples:

- database
- Kubernetes API
- S3
- remote configuration service

A Kubernetes ConfigMap mounted into the filesystem does not require Kubernetes-aware application code. It can still be consumed through `FileSourceDefinitionRepository`.

### 4.2 Schema/configuration abstraction

Answers:

> What does this source definition mean?

Flow:

```text
SourceDefinitionDocument
        |
        v
ConfigDecoder
        |
        v
Decoded document
        |
        v
Schema version validation / migration
        |
        v
Validation
        |
        v
SourceConfig
```

Version 1 encoding:

- YAML

Every definition contains:

```yaml
version: 1
```

Config schema version is separate from document revision.

### 4.3 Runtime abstraction

Answers:

> How is data collected from the configured source?

Flow:

```text
SourceConfig
    |
    v
SourceRunner
    |
    v
SourceAdapterRegistry
    |
    v
SourceAdapter
    |
    v
MonitorItem[]
```

Version 1 adapter:

`HtmlSourceAdapter`

## 5. Important Distinction: Schema Version vs Revision

These concepts must remain separate.

### Schema version

Example:

```yaml
version: 1
```

Meaning:

> Which config structure and semantics are being used?

### Definition revision

Example:

```text
revision = "abc123"
```

Meaning:

> Has this particular source definition changed?

Possible revision implementations:

- content hash
- file mtime
- database row version
- Kubernetes resourceVersion
- S3 ETag

The exact revision strategy for files is not yet fixed.

## 6. Stable Boundary

The most important source-side runtime contract is:

```text
SourceAdapter -> MonitorItem[]
```

Source-specific details must be normalized before data moves deeper into the system.

Future changes such as:

- HTML -> RSS
- file definitions -> database definitions
- HTTP -> browser automation
- YAML -> JSON

must not require redesigning downstream monitoring logic.

## 7. Version 1 Runtime

The version 1 runtime is intentionally narrow:

```text
File source definitions
        |
        v
YAML decoder
        |
        v
Schema v1 validation
        |
        v
SourceConfig(type=html)
        |
        v
HtmlSourceAdapter
        |
        +--> HTTP GET
        |
        +--> HTML parsing
        |
        v
MonitorItem[]
```

## 8. Async Direction

Source collection should use async I/O.

Expected implementation direction:

- `asyncio`
- `httpx.AsyncClient`

This is primarily because source monitoring is I/O-bound.

Concurrency policy, scheduling policy, retry policy, and per-domain rate limits are not yet fully specified and should not be over-designed during the first implementation.

## 9. Configuration Reload Direction

The architecture should not assume source definitions are loaded only once forever.

`SourceDefinitionRepository` should expose a simple read/list capability that may be invoked repeatedly.

Do not add watch/subscription APIs yet.

Future implementations may use:

- polling
- repository revisions
- database updates
- ConfigMap watches

The current contract should not prevent these evolutions.

## 10. Explicitly Unresolved Areas

The following must remain open until separately designed:

- exact `MonitorItem` schema
- item identity rules
- canonical URL rules
- content hashing
- first-run baseline behavior
- NEW / UPDATED / UNCHANGED semantics
- deletion semantics
- persistence
- scheduling
- retries and backoff
- rule engine
- notification model
- notification reliability/outbox semantics

Subagents must not silently lock these into permanent architecture.
