# Source Architecture v1

## 1. Objective

Implement a simple native Go source pipeline while preserving future extension to:

- new storage backends,
- new config encodings,
- new schema versions,
- new Go adapters,
- specialized adapters written in other languages.

Version 1 is:

```text
File + YAML + HTML + Go
```

## 2. Repository Boundary

Conceptual Go contract:

```go
type SourceDefinitionRepository interface {
    List(ctx context.Context) ([]SourceDefinitionDocument, error)
}
```

Version 1:

```text
FileSourceDefinitionRepository
```

Responsibilities:

- discover source definition files,
- read bytes,
- preserve origin information,
- provide a revision if practical.

Must not:

- parse YAML,
- validate source semantics,
- instantiate adapters,
- perform source HTTP requests.

## 3. Source Definition Document

Conceptual model:

```go
type SourceDefinitionDocument struct {
    Key      string
    Content  []byte
    Revision string
    Origin   string
}
```

The exact representation may be adjusted during implementation.

## 4. Config Decoder

Conceptual contract:

```go
type ConfigDecoder interface {
    Decode(
        document SourceDefinitionDocument,
    ) (DecodedConfig, error)
}
```

Version 1:

```text
YAMLConfigDecoder
```

Repository and decoder must remain independent.

## 5. Config Schema v1

Every definition contains:

```yaml
version: 1
```

Recommended v1 example:

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

Adapter-specific fields remain under `config`.

The generic source model must not expose HTML selector fields directly.

## 6. SourceConfig

Conceptual shape:

```go
type SourceConfig struct {
    ID              string
    Name            string
    Type            string
    IntervalSeconds int
    Config          map[string]any
}
```

The concrete implementation may use a safer intermediate structure if preferred.

Persistence metadata does not belong in `SourceConfig`.

## 7. Adapter-Specific Settings

The HTML adapter should translate the generic `Config` block into typed settings.

Conceptually:

```go
type HTMLSourceSettings struct {
    URL             string
    ItemSelector    string
    TitleSelector   string
    URLSelector     string
    ContentSelector string
}
```

Optional fields may use pointers or empty values according to idiomatic Go.

## 8. SourceAdapter

Core Go contract:

```go
type SourceAdapter interface {
    Collect(
        ctx context.Context,
        source SourceConfig,
    ) ([]MonitorItem, error)
}
```

Keep the interface small.

Do not add methods for:

- authentication,
- pagination,
- rate limiting,
- retry,
- token refresh,
- browser rendering,
- external transport.

These are implementation concerns or future requirements.

## 9. Adapter Registry

The registry resolves:

```text
html -> HTMLSourceAdapter
```

Future:

```text
rss -> RSSSourceAdapter
json -> JSONAPISourceAdapter
browser -> ExternalAdapter
```

Avoid spreading type-specific branching through application code.

## 10. SourceRunner

Conceptual responsibility:

```go
type SourceRunner struct {
    registry SourceAdapterRegistry
}
```

Runtime flow:

```text
SourceConfig
    |
    v
registry lookup
    |
    v
SourceAdapter.Collect
    |
    v
MonitorItem[]
```

The runner must remain transport-agnostic.

## 11. Native HTML Adapter v1

Version 1 supports:

- GET,
- static/server-rendered HTML,
- CSS selectors,
- repeated item nodes,
- title extraction,
- optional URL extraction,
- optional content extraction,
- relative URL resolution.

A focused HTML parsing library may be used.

The exact library is an implementation choice.

## 12. HTML Internal Structure

Recommended internal boundary:

```text
HTMLSourceAdapter
    |
    +--> HTTPFetcher
    |
    +--> HTMLExtractor
```

This allows parsing tests without live network requests.

Use Go's `context.Context` for cancellation.

## 13. Provisional MonitorItem

The final `MonitorItem` contract has not yet been designed.

For v1, use the smallest reversible model required to represent extracted output.

Likely provisional fields:

```go
type MonitorItem struct {
    SourceID string
    Title    string
    URL      string
    Content  string
}
```

Do not introduce permanent:

- item IDs,
- hashes,
- version fields,
- event types.

Those belong to later architecture work.

## 14. Polyglot Extension Point

A future external adapter must still satisfy the logical contract:

```text
SourceConfig -> MonitorItem[]
```

The Go interface is not itself the cross-language protocol.

Do not confuse:

```text
Go interface
```

with:

```text
language-neutral adapter protocol
```

The language-neutral protocol will be designed later.

## 15. Package Direction

A reasonable Go layout is:

```text
internal/
├── config/
│   ├── model.go
│   ├── repository.go
│   ├── file_repository.go
│   ├── decoder.go
│   └── yaml_decoder.go
│
├── source/
│   ├── model.go
│   ├── adapter.go
│   ├── registry.go
│   ├── runner.go
│   └── html/
│       ├── adapter.go
│       ├── settings.go
│       ├── fetcher.go
│       └── extractor.go
```

This is guidance, not a rigid requirement.

## 16. Testing

Use:

- table-driven tests,
- local fixtures,
- `httptest.Server` where network behavior is required.

Tests should cover:

- valid/invalid YAML,
- schema version handling,
- file discovery,
- registry behavior,
- unsupported types,
- HTML extraction,
- multiple items,
- optional content,
- optional URL,
- relative URLs,
- no matches,
- malformed expected item content,
- context cancellation where practical.
