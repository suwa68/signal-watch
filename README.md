# SignalWatch

A configurable source-monitoring system designed to detect meaningful changes in external information and send notifications through channels such as Telegram.

The initial scope covers loading and validating YAML source definitions and extracting static HTML into a common `MonitorItem` model. Change detection, alert rules, and notification delivery are planned for later stages.

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

Once Go packages have been added, run the test suite:

```sh
docker compose run --rm go go test ./...
```

Docker Compose keeps the Go module and build caches in named volumes between runs.
