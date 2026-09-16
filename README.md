# SignalWatch

A configurable source-monitoring system designed to detect meaningful changes in external information and send notifications through channels such as Telegram.

The current scope covers loading and validating YAML source definitions,
extracting static HTML into a common `MonitorItem` model, establishing a
process-local initial baseline, identifying unseen items, and sending completed
`Notification` values to Telegram. Producing notifications from source items,
durable state, richer change detection, and alert rules remain future work.

See the [architecture documentation](doc/architecture.md) for details.

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

## Telegram destination v1

Telegram delivery uses one bot per process and accepts multiple destinations.
It renders escaped HTML, truncates long summaries, disables link previews, and
sends once per notification.

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
