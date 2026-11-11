# CI reliability

The `CI` workflow runs on every pull request and on pushes to `main`. Its
`Test and vet` and `Monitoring acceptance` jobs are mandatory workflow jobs (no
manual dispatch or continue-on-error). `Test and vet` includes:

```sh
go test -count=1 -timeout=5m ./...
CGO_ENABLED=1 CC=gcc go test -race -count=1 -timeout=5m ./...
go vet ./...
```

The workflow uses Go 1.25 on an Ubuntu runner. Race instrumentation explicitly
enables CGO and selects the runner's GCC compiler; it must not use the tunnel
image's `CGO_ENABLED=0` build setting. Both test commands bypass cached test
results and give each package a five-minute test timeout. The whole job has a
15-minute limit, including checkout, toolchain setup, compilation, and vet.
New runs cancel superseded runs on the same pull request or branch. Workflow
permissions are read-only, and checkout does not retain Git credentials.

`Test and vet` retains its check name so existing branch-protection rules can
continue to require it. Repository administrators control branch protection;
the workflow itself does not configure those rules.

## Reproduce required validation locally

The repository's development container includes Go and GCC, so no host Go
installation is needed. From the checkout root, run:

```sh
docker compose --env-file /dev/null run --rm -T go go test -count=1 -timeout=5m ./...
docker compose --env-file /dev/null run --rm -T -e CGO_ENABLED=1 -e CC=gcc go go test -race -count=1 -timeout=5m ./...
docker compose --env-file /dev/null run --rm -T go go vet ./...
```

These tests use local fixtures and HTTP servers. They do not require Telegram
credentials or fetch monitored public sources. The tunnel integration test is
skipped by the ordinary Go suite because it requires its dedicated image.

## Monitoring acceptance

`Monitoring acceptance` runs `integration/monitor/acceptance.sh` on Ubuntu with a
15-minute job timeout. It builds the unchanged production monitor and one test
fixture, then forbids pulls/builds during the isolated runtime scenarios. It
requires Docker Compose, public base-image/module availability at build time,
and no host Go, credentials, live sources, host ports or mounted Docker socket.
The job retains read-only permissions and checkout `persist-credentials: false`.
Evidence is uploaded for seven days, including on failure; it contains synthetic
credentials only. Every PR head needs its own successful results.

The script checks fresh-volume ownership, actual SDK/TLS/renderer delivery,
retained silent/empty baselines, failure continuation/recollection, deduplication,
source independence, signal shutdown, ownership rejection, committed SIGKILL
recovery and uncertain-send termination/recollection. See [exact barriers and limits](monitor-compose.md#offline-acceptance).
It has bounded waits and project-scoped cleanup. A failed build is a failed job,
not skipped acceptance. The ordinary Go suite separately tests fixture contracts;
its tunnel integration package remains intentionally skipped without its image.

The check name is available for repository administrators to require in branch
protection. This change does not modify branch-protection settings or the existing
optional tunnel workflow/runtime.

## Optional Docker tunnel smoke

`Optional tunnel smoke` is a separate, manual-only `workflow_dispatch` workflow.
After the workflow is available on the default branch, select it in GitHub
Actions, choose **Run workflow**, and select the branch to validate. It runs:

```sh
docker compose --env-file /dev/null -f compose.tunnel.yaml run --build --rm -T --no-deps tunnel-smoke
```

The same command works locally. An empty Compose environment file prevents
automatic loading of a local `.env`. The selected `tunnel-smoke` service does
not inject runtime tunnel credentials, start the `tunnel` service, or mount
production source definitions. Its container has external networking disabled.
The test starts a local tunnel-client proxy and local HTML fixture, discovers
the two read-only MCP tools, calls `list_sources` and `collect_source`, and
checks the tool error for an unknown source. No hosted tunnel, real monitored
source, Telegram message, or secret is involved.

Building still needs access to Go dependencies, Docker base images, and the
pinned public tunnel-client image in GHCR. A registry outage or missing image
can therefore prevent smoke execution before any test runs. Keeping this job
manual avoids making that extra registry dependency part of required PR CI;
its failures still fail the manual run. Required CI itself still needs GitHub
Actions and Go dependency/toolchain availability.

The smoke job has the same read-only permissions, checkout credential policy,
15-minute job limit, and superseded-run cancellation as required CI. Its
existing test executable also has a 60-second test timeout. A successful local
smoke validates the container and fixture path; it does not establish hosted
ChatGPT access or live website collection.
