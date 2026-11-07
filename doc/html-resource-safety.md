# HTML collection resource limits

The default HTML adapter keeps static-page collection bounded without changing
the source v1 YAML schema or the `New(client)`, `NewAdapter`, `Collect`, `Fetcher`,
and `Extractor` signatures.

Each collection has these fixed safeguards:

| Resource | Limit | Behavior at the boundary |
| --- | --- | --- |
| Response body | 4 MiB (4,194,304 bytes) | Exactly 4 MiB is accepted; one extra byte fails the entire collection. |
| Matched items | 1,000 | Exactly 1,000 is accepted; more matches fail the entire collection. |
| Collection context | 15 seconds | The adapter derives a deadline from the caller context; an earlier caller deadline still wins. |

The adapter reads at most the byte limit plus one detection byte before parsing
HTML. It checks the actual stream rather than trusting `Content-Length`, so
unknown-length and chunked responses receive the same protection. With the
standard HTTP transport's automatic gzip decompression, the limit counts
decompressed bytes supplied to the adapter. A custom HTTP transport remains
responsible for any content-encoding handling it replaces.

The byte limit bounds the document supplied to the parser; it is not a hard heap
allocation cap. The default extractor checks the number of CSS item matches
before allocating monitor items. It does not truncate the response or returned
item list to fit a limit. Oversize, read, and extraction failures return an error
and no items, even when the response starts with valid items. Existing application
orchestration consequently does not establish or update its baseline for that
failed collection. Query output limits remain a separate presentation concern.

Response bodies close after success, parsing/read failure, limit overflow, or
cancellation. Non-2xx responses close immediately without draining the rejected
body. This may prevent connection reuse for those responses, but avoids waiting
for an irrelevant error stream. Read and cancellation errors preserve their
underlying causes for `errors.Is`; errors also identify the source and collection
stage.

The collection context covers fetching and response reading. The HTTP client must
honor request cancellation, as the standard Go transport does. A custom fetcher
must honor its supplied context and return a closable body whose reads can be
interrupted by cancellation. In-process parsing and extraction are synchronous;
their bounded input limits work, and cancellation is checked around these stages,
but a context deadline cannot forcibly interrupt arbitrary parser or custom
extractor code. These are per-collection safeguards, not process-wide concurrency
or admission controls.

The limits are intentionally fixed conservative defaults for static news/listing
pages. There are no new source settings, retries, scheduler behavior, state
semantics, or external-adapter protocol. A source requiring larger pages or more
items needs an explicit follow-up design change rather than silent truncation.

Local verification uses `resource_safety_test.go`, the existing HTML tests, and
the application's collection-failure baseline tests. All HTTP tests use
`httptest.Server` or a local transport double; no live website or Telegram
credentials are required.
