# SignalWatch and ChatGPT: Local MCP Integration

Updated: September 28, 2026 (Asia/Taipei)

## Goal and result

The goal was to let ChatGPT call SignalWatch MCP tools running in local Docker through OpenAI Secure MCP Tunnel. The first milestone was to call `list_sources` before connecting a real website.

**The first milestone is complete.** In a new ChatGPT Work conversation with `@SignalWatch` selected, `list_sources` returned:

| ID | Name | Type |
| --- | --- | --- |
| `example_news` | Example News | `html` |

This confirms that the **ChatGPT → Tunnel → local SignalWatch → source configuration** path worked at the time of the test. Collection from a real website has not yet been verified. The example URL, `example.com/news`, is a placeholder.

## What we completed

1. Created a Tunnel in OpenAI Platform, obtained its `tunnel_id`, and associated it with the intended ChatGPT workspace.
2. Created a runtime API key under a project in the same Platform organization for the local `tunnel-client`. Tunnels are organization scoped and have no project selection.
3. Configured a local `.env.tunnel.local` file with the Tunnel ID, API key, and example source directory. No real key is included in this document.
4. Started the local Docker-based Tunnel and SignalWatch setup, then created a personal `SignalWatch` plugin in ChatGPT with a Tunnel connection.
5. Selected `@SignalWatch` in a new Work conversation and received the `list_sources` result above.

During setup, the Developer mode switch described in the Plugins documentation did not appear under ChatGPT's Security and login settings. The Plugins creation page was available, and the successful tool call verified access in practice.

The plugin creation form initially selected OAuth and reported that OAuth settings could not be discovered. SignalWatch's stdio server does not implement OAuth, so the connection uses **No Authentication**. The local runtime API key authenticates tunnel-client to OpenAI and is separate from this plugin authentication setting.

## Local configuration and commands

Run these commands from your own SignalWatch repository. The path and values below are placeholders. Do not commit the runtime API key to Git.

```bash
cd /path/to/signal-watch
cp -n examples/tunnel.env.example .env.tunnel.local
chmod 600 .env.tunnel.local
nano .env.tunnel.local
```

Relevant fields in `.env.tunnel.local`:

```ini
CONTROL_PLANE_TUNNEL_ID=tunnel_your_actual_id
CONTROL_PLANE_API_KEY=your_actual_api_key
SIGNALWATCH_SOURCES_DIR=./examples/sources
```

With Docker or Rancher Desktop running:

```bash
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml run --rm tunnel doctor --explain
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml up -d tunnel
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml ps
docker compose --env-file .env.tunnel.local -f compose.tunnel.yaml logs --tail=80 tunnel
```

`doctor` checks configuration. Tool calls also require the local `tunnel-client` to remain running. If connectivity later fails, check container status and logs, then rerun `doctor`. Do not share the environment file or unredacted sensitive logs.

## Using the plugin

1. Confirm that `SignalWatch` has been created and installed in [ChatGPT Plugins](https://chatgpt.com/plugins).
2. Start a new ChatGPT Work conversation and select `@SignalWatch`.
3. Test prompt: `Call list_sources to show the configured sources. Do not use web search.`
4. The current expected result includes `example_news`. After changing local source definitions, restart the service that loads them. If MCP tool metadata changes, refresh the connection in ChatGPT and test again in a new conversation.

## Access and distribution

This is a **personal development plugin**. Publishing the SignalWatch GitHub repository does not automatically make the plugin available to other ChatGPT users. The Tunnel does not expose the local MCP server as a public URL. Access depends on associated organization and workspace permissions, and the local client must be running.

Public distribution requires a separate plugin submission and review process with a stable, publicly reachable HTTPS MCP endpoint. Secure MCP Tunnel cannot serve as the endpoint for public plugin submission.

## Next steps

1. Choose a **publicly testable** real source and add a SignalWatch source definition matching the project's schema. Public examples should contain only non-sensitive, reproducible source configurations.
2. Restart the service that loads source definitions. Call `list_sources` to confirm the new source ID appears.
3. Call `collect_source` and inspect extracted titles, URLs, item count, empty results, and errors. Only then will collection from a real source be verified end to end.
4. Validate additional source types and downstream analysis capabilities separately. A successful `list_sources` call verifies catalog access, not data collection or analysis capabilities.

## References

- [OpenAI: Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)
- [OpenAI: Connect and test your plugin](https://developers.openai.com/plugins/deploy/connect-chatgpt)
- [OpenAI: Plugins quickstart](https://developers.openai.com/plugins/quickstart)
- [SignalWatch public repository](https://github.com/suwa68/signal-watch)
