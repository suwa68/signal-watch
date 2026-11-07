# Real source verification

The bundled `examples/sources/signalwatch-commits.yaml` configures the existing
native HTML adapter for the public, server-rendered GitHub commit page at
<https://github.com/suwa68/signal-watch/commits/main>. No authentication,
browser rendering, pagination, or additional adapter is configured. Its
`interval_seconds: 300` is source metadata; these query commands do not start
a scheduler.

## Provenance and verification boundary

[PR #16](https://github.com/suwa68/signal-watch/pull/16) proposed this source
on `feat/live-html-source`, head `4da9944`, based on main `6950ade`. It was
closed without merging. The main checkout at `6950ade` contains only the
placeholder source. This change selectively restores the exact YAML from
that PR; it does not merge its branch. The original PR changed the YAML and
README only, so it supplies no captured HTML fixture to restore.

PR #16's description records a historical live GET yielding eight rows and
passing CI, while leaving hosted ChatGPT collection unverified. That report
is historical evidence, not a fresh live collection in this change. Its
referenced issue #15 is deleted (GitHub returned HTTP 410 when inspected on
2026-11-08). The restoration and deterministic verification are tracked in
[issue #21](https://github.com/suwa68/signal-watch/issues/21).

`internal/source/html/testdata/real_source_commits.html` is a small,
**reconstructed structural fixture**, created on 2026-11-08 from PR #16's
selectors. It is not a captured or independently checked current GitHub
response. The first two commit titles and full SHAs come from local main's
Git history at `6950ade`:

| Commit | Title |
| --- | --- |
| `6950ade914918082c67aa6f08fda3e7a2aab862a` | Run SignalWatch MCP tunnel entirely in Docker (#14) |
| `f60075c75992a2ba499a3ad3ce352bfade870fd2` | feat: add AI-callable source Query v1 (#12) |

The fixture keeps only the row/title/link structure needed by the selectors.
Whitespace, nested title text, navigation, author and SHA links, unrelated
metadata, and the illustrative time element are synthetic test cases.
Additional inline synthetic cases represent no matching rows and a row
missing its required title. These cases test the committed selector
configuration against known input; they cannot detect upstream markup drift.

## Deterministic automated coverage

`real_source_test.go` loads the committed example directory through the file
repository, YAML decoder, schema validation, and HTML catalog validation.
It resolves `signalwatch_commits` by ID, then runs the existing registry,
`SourceRunner`, HTTP fetcher, and extractor. A separate assertion uses the
same production Query service assembly.

The client has a fixture-only `http.RoundTripper`: it accepts only GET for
the configured public URL and has no fallback network transport. The URL
and selectors are never rewritten in the test. Assertions check:

- Two exact titles in page order, excluding navigation/author/SHA decoys.
- Root-relative and absolute hrefs becoming absolute GitHub commit URLs.
- Empty optional content, external ID, and publication time; Query reports
  `content_status: empty` and `content_completeness: unknown`.
- A valid empty collection when no rows match.
- A missing title failing the complete collection without partial items.

Run the focused check and the surrounding suites with Docker:

```sh
docker compose run --rm -T go go test ./internal/source/html -run '^TestRealSourceCommittedYAML' -count=1 -v
docker compose run --rm -T go go test ./internal/config ./internal/source/... ./internal/query/... -count=1
docker compose run --rm -T go go test ./... -count=1
docker compose run --rm -T go go test -race ./internal/config ./internal/source/... ./internal/query/... -count=1
docker compose run --rm -T go go vet ./...
```

All automated collection uses local fixtures or local test servers. No seen
state, Telegram delivery, tunnel connection, credentials, or external network
collection is required by these tests.

## Remaining manual live acceptance

This change does not claim a fresh live website or hosted ChatGPT success.
When performing live acceptance separately:

1. Run the README's local `source-query` command for `signalwatch_commits`.
   Confirm at least one current commit title and an absolute URL under
   `https://github.com/suwa68/signal-watch/commit/`; compare them with the
   public page. An empty result is possible when the HTML changes and does
   not count as successful real-source acceptance.
2. Ensure the existing MCP deployment mounts the updated source directory
   and reload its catalog through its normal restart procedure. The MCP
   catalog is loaded once at startup. See [Docker MCP Tunnel
   setup](docker-mcp-tunnel.md) for deployment instructions.
3. In ChatGPT with the intended SignalWatch connection, call `list_sources`
   and confirm `signalwatch_commits`, then call `collect_source` with that
   ID. Check nonempty current titles and absolute commit URLs. Record the
   date, item count, and representative non-sensitive title/URL as evidence.

The source intentionally omits content and publication time selectors;
it does not provide full commit messages or dates. GitHub can change its
public markup or availability. A successful fixture check, local live CLI
query, and hosted ChatGPT collection are three separate pieces of evidence.
