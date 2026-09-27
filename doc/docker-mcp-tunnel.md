# SignalWatch through Secure MCP Tunnel in Docker

```text
ChatGPT -> OpenAI Tunnel endpoint
                     ^ outbound HTTPS from the container
              tunnel-client
                     | child process stdin/stdout
              signalwatch-mcp -> configured source websites
```

Both executables run in one Linux container. Docker builds the Go executable
for the target Linux architecture, so macOS does not need Go, tunnel-client,
or a native SignalWatch binary. The official tunnel image supports Linux arm64
and amd64. The Dockerfile pins its release and multi-platform image digest;
SignalWatch continues to use its existing Go 1.25 toolchain.

This deployment is for private developer-mode access. It does not introduce
an HTTP MCP server, scheduler, notification delivery, or public plugin release.

## 1. Build and validate without credentials

Run from the repository root with Docker running:

```sh
docker compose -f compose.tunnel.yaml build tunnel
docker compose -f compose.tunnel.yaml run --build --rm tunnel-smoke
```

The smoke image includes a Go test executable; the runtime image does not.
The test starts the official `tunnel-client dev proxy`, which provides a local
control plane and runs the tunnel client against the real SignalWatch stdio
child. A real MCP client connects to the local proxy to check:

- MCP initialization and discovery of exactly the two read-only tools;
- `list_sources` with a generated source catalog;
- `collect_source` with expected title, content, and resolved URL;
- a tool error for an unknown source ID.

The smoke container has `network_mode: none`: all requests use loopback and a
local HTML fixture. Image builds download dependencies, but test execution
uses no external sites, API keys, OpenAI account, or Telegram.

Normal Go tests remain available through the existing development service:

```sh
docker compose run --rm -T go go test ./...
```

The Docker-specific test skips in this normal suite. The explicit smoke command
above is required to validate the actual packaged executables.

## 2. Create the OpenAI Tunnel

Follow the official [Secure MCP Tunnel guide](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels).
In Platform tunnel settings, create a tunnel, record its `tunnel_id`, and obtain
a runtime API key. Creating/editing requires Tunnels Read + Manage;
running/selecting requires Read + Use. Associate the tunnel with the intended
ChatGPT workspace as well as its owning Platform organization.

ChatGPT developer mode is a separate account/workspace capability. Enable it
under Settings -> Security and login when available. See the official
[developer-mode guide](https://developers.openai.com/api/docs/guides/developer-mode).

## 3. Configure locally

Copy the template once (do not overwrite an existing local configuration):

```sh
cp -n examples/tunnel.env.example .env.tunnel.local
chmod 600 .env.tunnel.local
```

Edit `.env.tunnel.local` on your computer:

```dotenv
CONTROL_PLANE_TUNNEL_ID=tunnel_your_id
CONTROL_PLANE_API_KEY=your_runtime_api_key
SIGNALWATCH_SOURCES_DIR=/absolute/host/path/to/source-definitions
```

The file is ignored by Git and excluded from the Docker build context. Do not
paste the API key into chat or put it in a Dockerfile/build argument. Compose
injects it into the container environment only at runtime. Use `config --quiet`
for configuration validation; plain `docker compose config` expands secrets.

`SIGNALWATCH_SOURCES_DIR` is a host directory, mounted read-only at `/sources`.
It must already exist and contain valid YAML definitions readable by container
UID 10001. Avoid putting unrelated or draft YAML files in this directory:
SignalWatch validates the whole catalog before starting.

The default `./examples/sources` is sufficient for `list_sources`. Its
`example.com/news` URL is a placeholder and is not a real collection target.
Use a real supported static HTML source for hosted collection acceptance.

The image sets `MCP_COMMAND` to:

```text
/usr/local/bin/signalwatch-mcp --sources-dir /sources
```

This uses tunnel-client's documented environment-based configuration instead
of generating a persistent profile with `init`. There is no profile volume or
extra host installation. Query collection keeps its default 15-second timeout.

## 4. Diagnose and start

Validate configuration and run the official diagnostics:

```sh
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml config --quiet
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml run --rm tunnel doctor --explain
```

Start in the foreground for the first hosted test:

```sh
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml up tunnel
```

Or start in the background:

```sh
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml up -d tunnel
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml logs --tail=100 -f tunnel
```

Keep only one active tunnel-client for a given tunnel ID. The upstream stdio
transport does not support multiple replicas sharing that ID. Stop the
foreground instance before switching to a background instance. The container
uses Docker init and `restart: unless-stopped`; it still depends on the host
and Docker engine staying awake and running.

Health endpoints remain on loopback inside the container, with no published
host port. The Compose healthcheck uses `/readyz`. Inspect readiness with:

```sh
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml ps
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml exec tunnel \
  wget -q -O - http://127.0.0.1:8080/readyz
```

Readiness is diagnostic evidence, not proof that ChatGPT can discover or call
the tools. Do not enable remote admin UI access just to connect ChatGPT;
the hosted connection uses the outbound tunnel path.

## 5. Accept from ChatGPT

Follow [Connect and test your plugin](https://developers.openai.com/plugins/deploy/connect-chatgpt):

1. Open ChatGPT Plugins with developer mode enabled and add a connection.
2. Choose **Tunnel** and select the configured tunnel or enter its ID.
3. Set **Authentication** to **No Authentication**. This SignalWatch stdio
   server does not implement OAuth. The runtime API key authenticates
   tunnel-client to OpenAI; it is not an OAuth credential for this form.
4. Check that discovery returns `list_sources` and `collect_source`.
5. Start a new conversation and add the SignalWatch connection.
6. Ask it to use SignalWatch's `list_sources`, then call `collect_source` with
   an actual returned source ID and inspect the returned items.

Success means the ChatGPT conversation receives both tool results through the
running hosted tunnel. A passing local smoke test, successful build, or healthy
container alone does not establish hosted acceptance.

The [September 28 setup record](signalwatch-chatgpt-tunnel-setup-2026-09-28.md)
records successful hosted `list_sources` acceptance. In that account, the
Developer mode switch was absent but plugin creation and calling worked.
Real-source collection has not yet been verified through ChatGPT.

## Operations and troubleshooting

- After editing source YAML, restart the container because the catalog loads
  once at MCP startup:

  ```sh
  docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml restart tunnel
  ```

- After editing credentials or the mounted directory setting, recreate it:

  ```sh
  docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml up -d --force-recreate tunnel
  ```

- Stop the deployment with:

  ```sh
  docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml down
  ```

- For a missing tunnel in ChatGPT, check the workspace association and Tunnels
  Use permission. For discovery failures, check container logs, source catalog
  validation, and `doctor --explain`. For collection failures, check the source
  URL/selectors and outbound access to that website from the container.
- If the creation form reports that it cannot discover OAuth settings, change
  Authentication from OAuth to No Authentication for this server.
- No inbound internet ports are needed. The runtime needs outbound HTTPS to
  OpenAI and access to each configured source website.
- For tunnel-client upgrades, consult the official
  [latest release](https://github.com/openai/tunnel-client/releases/latest),
  update the tag and multi-platform digest in `Dockerfile.tunnel`, and rerun
  the Docker smoke test before replacing the running container.
