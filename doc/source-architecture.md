# Source Architecture v1

## 1. Objective

Source v1 should be simple enough to implement quickly while preserving clear extension points for:

- new config storage backends,
- new config encodings,
- new config schema versions,
- new source collection mechanisms.

Version 1 only needs filesystem + YAML + static HTML.

## 2. Source Definition Repository

### Contract

Conceptually:

```python
from typing import Protocol


class SourceDefinitionRepository(Protocol):
    async def list(self) -> list["SourceDefinitionDocument"]:
        ...
```

The exact method name may be adjusted for Python conventions, but the responsibility must remain narrow.

### Version 1 implementation

`FileSourceDefinitionRepository`

Responsibilities:

- discover source-definition files in a configured directory,
- read their raw content,
- return `SourceDefinitionDocument` objects,
- attach enough origin information for useful errors,
- optionally calculate a revision.

Must not:

- parse YAML semantics,
- validate HTML selectors,
- instantiate source adapters,
- perform network requests.

## 3. Source Definition Document

Conceptual model:

```python
from dataclasses import dataclass


@dataclass(frozen=True)
class SourceDefinitionDocument:
    key: str
    content: str
    revision: str | None = None
    origin: str | None = None
```

`origin` is useful for diagnostics such as:

```text
/path/to/sources/murata.yaml
```

This model is infrastructure-neutral.

## 4. Config Decoder

### Contract

A decoder converts serialized content into a generic decoded representation.

Conceptually:

```python
class ConfigDecoder(Protocol):
    def decode(self, document: SourceDefinitionDocument) -> object:
        ...
```

Version 1:

`YamlConfigDecoder`

Do not couple `FileSourceDefinitionRepository` directly to PyYAML.

## 5. Schema Version

Every v1 source config must contain:

```yaml
version: 1
```

Unknown versions must fail clearly.

The initial implementation does not need a sophisticated migration framework.

A small dispatcher/validator is sufficient, for example conceptually:

```text
version == 1 -> parse v1
otherwise -> UnsupportedConfigVersion
```

The architecture should leave room for future migration/version handlers.

## 6. Generic Runtime Configuration

The top-level runtime model should remain source-type-neutral.

Conceptual shape:

```python
@dataclass(frozen=True)
class SourceConfig:
    id: str
    name: str
    type: str
    interval_seconds: int
    config: Mapping[str, object]
```

The exact Python types may be improved during implementation.

Important:

- HTML selectors must not become permanent top-level fields of generic `SourceConfig`.
- adapter-specific configuration belongs under `config`.
- persistence metadata must not be added to this model.

## 7. HTML Adapter Settings

`HtmlSourceAdapter` may validate/convert `SourceConfig.config` into an adapter-specific typed settings model.

Conceptually:

```python
@dataclass(frozen=True)
class HtmlSourceSettings:
    url: str
    item_selector: str
    title_selector: str
    url_selector: str | None = None
    content_selector: str | None = None
```

This gives type safety without making the generic config HTML-specific.

## 8. Source Adapter Contract

Conceptually:

```python
from typing import Protocol


class SourceAdapter(Protocol):
    async def collect(
        self,
        source: SourceConfig,
    ) -> list["MonitorItem"]:
        ...
```

Version 1 only implements:

`HtmlSourceAdapter`

This contract must stay small.

Do not add methods for:

- authentication,
- pagination,
- token refresh,
- retries,
- rate limiting,
- rendering,
- transformation pipelines.

Those concerns can be designed when real requirements appear.

## 9. Source Adapter Registry

Use a registry so source type resolution does not become a growing conditional.

Conceptual behavior:

```python
registry.register("html", html_adapter)
adapter = registry.get(source.type)
```

Registry requirements:

- explicit registration,
- clear error for unsupported type,
- no global mutable singleton required by domain code.

## 10. Source Runner

`SourceRunner` orchestrates adapter lookup and collection.

Conceptually:

```python
class SourceRunner:
    def __init__(self, registry: SourceAdapterRegistry):
        self._registry = registry

    async def run(self, source: SourceConfig) -> list[MonitorItem]:
        adapter = self._registry.get(source.type)
        return await adapter.collect(source)
```

It must not know about:

- BeautifulSoup,
- PyYAML,
- filesystem paths,
- database rows,
- Kubernetes.

## 11. HTML Source v1

Supported behavior:

1. perform HTTP GET,
2. parse returned HTML,
3. find item elements with a CSS selector,
4. extract title,
5. extract optional URL,
6. resolve relative URLs using the source page URL,
7. extract optional content,
8. return `MonitorItem[]`.

Recommended source definition:

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

The exact interpretation of a URL selector should be simple in v1:

- select an element,
- read its `href`.

Title and content selectors should extract normalized visible text.

Do not build a transformation DSL.

## 12. HTTP Fetching

The HTML implementation should keep HTTP access behind a small internal dependency so parsing can be tested independently.

Expected direction:

```text
HtmlSourceAdapter
    |
    +--> HttpFetcher
    |
    +--> HtmlExtractor
```

or an equivalent structure.

Version 1 HTTP scope:

- GET only
- reasonable timeout
- static/server-rendered HTML

Do not add authentication or browser rendering.

Exact retry policy is currently unresolved.

## 13. MonitorItem

`MonitorItem` is the output boundary of the source layer.

Its final schema is not yet fully designed.

For the current source implementation, create the smallest practical model needed to represent extracted items, but keep it easy to revise.

Likely fields may include:

- source identifier
- title
- URL
- content

Do not finalize permanent item identity or hashing semantics yet.

## 14. File Layout Direction

A reasonable starting point:

```text
app/
├── config/
│   ├── models.py
│   ├── repository.py
│   ├── file_repository.py
│   ├── decoder.py
│   └── yaml_decoder.py
│
├── source/
│   ├── models.py
│   ├── adapter.py
│   ├── registry.py
│   ├── runner.py
│   └── html/
│       ├── adapter.py
│       ├── settings.py
│       ├── fetcher.py
│       └── parser.py
```

This is guidance, not a mandatory package tree. Prefer clarity over mirroring this tree exactly.

## 15. Required Tests

At minimum:

### File repository

- reads supported definition files
- ignores unrelated files if appropriate
- produces useful origin information
- handles unreadable/invalid paths clearly

### YAML decoder

- valid YAML
- malformed YAML
- empty documents

### Schema v1

- valid v1 config
- missing version
- unsupported version
- missing required generic fields
- invalid adapter-specific config

### Adapter registry

- registered adapter lookup
- unsupported type

### HTML parser/adapter

- multiple items
- title extraction
- optional content
- optional URL
- relative URL resolution
- malformed/missing expected item fields
- no matching items

Do not use live websites in unit tests.
