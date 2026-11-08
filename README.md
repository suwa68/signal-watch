# SignalWatch

A configurable source-monitoring system designed to detect meaningful changes in external information and send notifications through channels such as Telegram.

The current scope covers loading and validating YAML source definitions,
extracting static HTML into a common `MonitorItem` model, establishing a
silent initial baseline, and delivering later unseen items to Telegram.
The monitor command now runs independent fixed-delay source loops with SQLite
state retained across restarts. The callable application pipeline creates
notifications from original item metadata with an empty summary and marks items
seen only after successful delivery. NLP, richer change detection, and alert
rules remain future work. A nonroot Docker Compose monitor retains state in a
named volume; offline acceptance exercises the unchanged production image.

See the [architecture documentation](doc/architecture.md) for details.

## Monitor command

`go run ./cmd/signalwatch --sources-dir /path/to/sources --state-dir /path/to/state`
starts the monitor using `SIGNALWATCH_TELEGRAM_BOT_TOKEN` and
`SIGNALWATCH_TELEGRAM_CHAT_ID`. `--sources-dir` is required; `--state-dir` defaults
to `/state`. Use a writable local directory retained across restarts, and keep the
Telegram destination stable for that state. Startup validates the complete HTML
catalog and intervals, acquires exclusive lifetime ownership of the state
directory, then initializes Telegram (`getMe`, bounded by the SDK's five seconds).
An empty catalog or invalid definition rejects the entire startup.

One Pipeline per source runs immediately, then waits `interval_seconds` after
completion. Sources progress independently. Collection retains its 15-second
HTTP bound; each send gets a 20-second child context. Normal item failures wait
for the next regular collection; there is no pending-item queue. A fatal state
failure cancels all workers after its Pipeline returns, possibly after remaining
items have been attempted.

The first successful collection, even empty, atomically establishes a silent
baseline. SQLite retains baseline and seen keys across restarts. A successful
send precedes the durable seen mark; an uncertain send or interrupted/failed
mark may duplicate on recollection. Disappearing failed items have no delivery
guarantee. Only one process may own a state directory; retain `state.db` and its
stable `.signalwatch.lock` together, without deleting the lock file.

SIGINT/SIGTERM cancel requests and timers. Workers finish before state closes.
A clean signal exits 0; runtime/state/cleanup failures or exceeding the fixed
10-second shutdown grace exit 1; invalid configuration exits 2. If a worker
ignores cancellation, timeout exits the process without closing its active store.
Startup and completed runs emit bounded JSON on stdout with `event`, `source_id`
and `status`; error records contain redacted, truncated details. Fatal command
errors go to stderr, with a 25-millisecond best-effort final flush so blocked
output cannot prevent process exit. A completed-run event follows the entire
Pipeline return, including baseline/mark work. Query CLI/MCP remain independent of this state.

## Docker monitor

Supply `SIGNALWATCH_SOURCES_DIR`, `SIGNALWATCH_TELEGRAM_BOT_TOKEN` and
`SIGNALWATCH_TELEGRAM_CHAT_ID`, then run:

```sh
docker compose -p signalwatch-monitor -f compose.monitor.yaml up -d --build
```

The production service uses a read-only root filesystem/source mount, a nonroot
owned state volume, automatic restart and a 15-second stop grace. Keep the same
project/volume across upgrades; deleting the volume resets monitoring state.
See [monitor Compose operations and offline acceptance](doc/monitor-compose.md)
for setup, lifecycle, failure assumptions and evidence limits. The deterministic
credential-free production-image check needs no host Go:

```sh
integration/monitor/acceptance.sh
```

## Query v1

Query v1 exposes current configured source data without reading or changing
monitoring state and without sending notifications. The JSON CLI loads and
validates every definition before collecting one exact source ID:

```sh
docker compose run --rm go go run ./cmd/source-query \
  --sources-dir ./examples/sources \
  --source-id example_news
```

The read-only MCP server loads the catalog once at startup and serves
`list_sources` and `collect_source` over stdio. Restart it after definition
changes:

```sh
docker compose run --rm -T go go run ./cmd/signalwatch-mcp \
  --sources-dir ./examples/sources
```

To use the Docker-hosted server with Codex CLI, register Docker as the stdio
command. Replace the repository path with this checkout's absolute path:

```sh
codex mcp add signalwatch -- \
  docker compose -f /absolute/path/to/signal-watch/compose.yaml \
  run --rm -T go go run ./cmd/signalwatch-mcp \
  --sources-dir /workspace/examples/sources
```

ChatGPT web is the intended recurring-analysis client. Its hosted environment
does not read local Codex MCP configuration; connect the server through a
developer-mode plugin and Secure MCP Tunnel when those account and workspace
capabilities are available. See the generic calling-AI instructions in
[`examples/ai/query-analysis.md`](examples/ai/query-analysis.md).

### ChatGPT through Secure MCP Tunnel (all Docker)

The tunnel client and SignalWatch run in one container and communicate over
stdio. The host needs Docker Compose; it does not need Go or tunnel-client.
No host ports or Docker socket are mounted into the container.

Build and validate locally without an API key or an OpenAI Tunnel:

```sh
docker compose -f compose.tunnel.yaml build tunnel
docker compose -f compose.tunnel.yaml run --build --rm tunnel-smoke
```

The smoke test runs the official tunnel client's local proxy, discovers and
calls both SignalWatch tools, and collects a local HTML fixture with external
networking disabled. This validates the container path; actual ChatGPT access
still requires a Platform Tunnel and ChatGPT developer-mode connection.

See [Docker MCP Tunnel setup](doc/docker-mcp-tunnel.md) for runtime credentials,
source mounts, startup, diagnostics, and ChatGPT acceptance steps.
The [September 28 setup record](doc/signalwatch-chatgpt-tunnel-setup-2026-09-28.md)
records a successful ChatGPT `list_sources` call through the hosted tunnel.
Collection from a real website remains the next acceptance step.

### Public HTML source example

[`signalwatch-commits.yaml`](examples/sources/signalwatch-commits.yaml) targets
this repository's public GitHub `main` commit page with the existing HTML
adapter. It extracts commit titles and links without credentials. The
`example_news` definition remains a schema example with a placeholder URL.

Run the deterministic check without contacting GitHub:

```sh
docker compose run --rm -T go go test ./internal/source/html \
  -run '^TestRealSourceCommittedYAML' -count=1 -v
```

To inspect current public HTML manually with the JSON CLI:

```sh
docker compose run --rm -T go go run ./cmd/source-query \
  --sources-dir ./examples/sources \
  --source-id signalwatch_commits
```

Fixture verification does not establish that GitHub's current markup still
matches or that ChatGPT can collect through the hosted tunnel. See
[real source verification](doc/real-source-verification.md) for fixture
provenance, output limitations, and the remaining manual acceptance steps.

## Development

The Go toolchain runs in Docker, so Go does not need to be installed on the host.

Pull the development image and verify the toolchain:

```sh
docker compose pull go
docker compose run --rm go go version
docker compose run --rm go go list -m
```

Open a shell in the development container:

```sh
docker compose run --rm go
```

Run the test suite:

```sh
docker compose run --rm go go test ./...
```

Docker Compose keeps the Go module and build caches in named volumes between runs.

An example version 1 source definition is available at
[`examples/sources/example.yaml`](examples/sources/example.yaml).

## Notification orchestration v1

`application.NewPipeline(collector, store, sender)` assembles the existing source
runner and baseline/deduplication processor with a bound notification sender.
Call `pipeline.RunOnce(ctx, sourceConfig)` repeatedly, reusing the pipeline and
store. The monitor binds SQLite; callable v1 examples use a memory store. The
first successful collection, including an empty collection, establishes a silent baseline. Later new items are delivered
sequentially with their original title, source name, optional URL and publication
time, and no summary.

Mapping, send, and seen-state write failures are retained in an aggregate error.
Later items are still attempted while the caller context remains active. Only
the caller's `ctx.Err()` stops new items; a sender's own timeout or cancellation
does not stop the run. Overlapping calls on one pipeline return
`application.ErrRunInProgress` before collection.

Run the credential-free integration scenario:

```sh
docker compose run --rm go go test ./internal/application -run TestPipelineFromYAMLToNotifications -v
```

It loads a temporary YAML definition, collects changing HTML from a local server,
and uses the real Telegram renderer with a recording sender. Repeated calls
demonstrate a silent A/B baseline, delivery of C from A/B/C, and suppression of C
on the next run. It sends no Telegram messages.

The memory-store examples lose state on restart and establish a new silent
baseline; the monitor's SQLite store retains it. Failed items can be tried on a
later call only if they are collected again with the same identity. There is no durable pending-item snapshot or delivery guarantee;
an uncertain send or failed state write can cause a duplicate. Keep the single
destination binding stable for the pipeline's lifetime.

See [Notification orchestration v1](doc/notification-orchestration-v1.md) for
assembly, error semantics, caller-driven runs, and limitations.

## Telegram destination v1

Telegram delivery uses one bot per process and accepts multiple destinations.
It renders escaped HTML, omits absent summaries, truncates long summaries,
disables link previews, and sends once per notification. Each application
pipeline binds one destination explicitly.

For a real smoke test, first create a bot and grant it channel posting permission.
Create `.env.telegram.local` in the project root (ignored by Git) and fill in:

```dotenv
SIGNALWATCH_TELEGRAM_BOT_TOKEN=
SIGNALWATCH_TELEGRAM_CHAT_ID=
```

Use Docker Compose's `--env-from-file` option to explicitly load the settings and
send one test notification:

```sh
docker compose run --rm \
  --env-from-file .env.telegram.local \
  go go run ./cmd/telegram-smoke
```

The file is not loaded automatically by the application. An alternative using
exported environment variables is documented in the Telegram v1 guide.

See [Telegram v1](doc/telegram-v1.md) for setup, the Go API, length handling, and
error behavior. Normal tests use fakes and local HTTP servers and never send to
Telegram.
