# Monitoring with Docker Compose

`Dockerfile.monitor` builds the ordinary `cmd/signalwatch` executable into a
scratch image containing the binary, CA certificates, and `/state` owned by
UID/GID `10001:10001`. Its Dockerfile-specific context allowlist excludes tests,
fixtures, source catalogs, hidden files, and local credentials. There is no shell,
SDK override, test endpoint, or separate production test binary in this image.

## Operator setup

Create a local directory of version 1 HTML source YAML files and set
`SIGNALWATCH_SOURCES_DIR` to its absolute path. All definitions and intervals are
validated at startup. Read-only mounts must be readable/traversable by UID 10001.
Set the existing `SIGNALWATCH_TELEGRAM_BOT_TOKEN` and
`SIGNALWATCH_TELEGRAM_CHAT_ID` in the calling environment or an ignored dotenv
file supplied explicitly with `--env-file`. Do not commit credentials or include
resolved production Compose output in public diagnostics: it contains the token.

```sh
# After supplying the three variables above:
docker compose -p signalwatch-monitor -f compose.monitor.yaml up -d --build
docker compose -p signalwatch-monitor -f compose.monitor.yaml logs --tail 50 monitor
docker compose -p signalwatch-monitor -f compose.monitor.yaml stop monitor
```

The production service runs as UID/GID 10001 with a read-only root filesystem,
read-only sources, dropped capabilities, no-new-privileges, Docker init, and a
writable `monitor-state` named volume. Docker initializes a genuinely new volume
from the image's owned `/state`; an existing incorrectly owned directory is not
silently repaired. The service uses `restart: unless-stopped` and a 15-second
stop grace around the command's 10-second shutdown grace. Permanent configuration
or state faults can therefore restart repeatedly; inspect the fatal logs and fix
the cause. There is no HTTP health endpoint, published port, or Docker socket.

Keep the same Compose project name and volume when upgrading or recreating the
container. `up -d --build --force-recreate monitor` preserves its named state;
`restart monitor` preserves both container and volume. A normal `down` preserves
named volumes; `down --volumes` deliberately destroys the baseline and seen keys.
Do not scale this service against one state directory. A second owner fails
before Telegram `getMe`. Do not remove `.signalwatch.lock` while an owner exists.
Use a local filesystem supporting SQLite and advisory locks. Take state copies
only with the monitor stopped; retain the state directory together. No migration,
backup service, or network filesystem guarantee is provided.

Keep source IDs, provisional item keys and the single destination binding stable
for retained state. Source definition changes require restart. The first
successful collection, including empty, is silent. Unseen items send before their
seen marks commit. A failed C does not block later D; C is retried only if the same
item is collected again. A disappearing failed item is not queued. Uncertain send
outcomes or failed/interrupted marks can duplicate. Clean signal shutdown exits 0;
fatal state/cleanup/shutdown-timeout errors exit nonzero.

## Offline acceptance

```sh
integration/monitor/acceptance.sh
```

Only Docker Compose and a Bash host are required. Builds may fetch the public Go
base image and modules; offline **runtime** starts forbid pulls/builds. Each run
owns a unique `sw-acceptance-*` project and cleans only its containers, internal
network and synthetic named volumes. It never prunes or removes unrelated
runtimes. Images/build caches remain available. Evidence defaults to ignored
`.superpowers/sdd/monitor-acceptance/<project>`; override
`SIGNALWATCH_ACCEPTANCE_LOG_DIR` to retain it elsewhere. Cleanup failures report
the exact owned project. There are no real credentials, live websites or real
Telegram messages.

The overlay supplies literal synthetic credentials, explicitly empty upper/lower
proxy variables, read-only fixture definitions and `SSL_CERT_FILE` pointing at an
ephemeral CA. One fixture serves HTML and TLS with DNS alias `api.telegram.org` on
an internal network; its control listener binds loopback only. Private keys stay
in fixture memory. Readiness checks both listeners and the actual TLS trust/name
handshake before the monitor starts. The CA is not regenerated during scenarios.
The resolved Compose model is checked for isolation, mounts, credentials and the
unchanged production entrypoint. Only the test overlay disables automatic restart
so stop/kill/recreate events are deterministic; production restart policy is also
checked from its separately resolved model.

The script tests the unchanged production image's actual SDK `getMe` and multipart
`sendMessage`, exact escaped HTML renderer output, chat, parse mode and disabled
previews. One-shot held HTML responses control runs. For successful notifications,
it holds the complete Telegram response, snapshots the source's completed-run
log cursor, releases the response, then waits for a later successful completed
run. Polls have deadlines; elapsed time alone never establishes completion. Log
reads are snapshots, with no background log-follow process. The second-owner
subprocess is bounded and reaped on success, failure and signal.

Assertions cover fresh-volume ownership, silent A/B and empty baselines, retained
state after container recreation and ordinary restart, failed C/successful D and
C recollection, deduplication, and independent source progress during a held request.
The independence barrier also rejects a timeout/cancellation followed by a
replacement held request. Assertions include SIGTERM cancellation/exit 0,
ownership rejection before getMe, and SIGKILL exit 137 followed by committed-state recovery. A profile-gated inspector mounts the
same state volume read-only with copy-up disabled, refuses live ownership via a
nonblocking lock, opens SQLite `mode=ro`, and asserts exact initialized/seen sets.
It cannot initialize, migrate, repair, or chown the production state directory.
The harness also proves that live inspection is refused.

This production-image evidence is ordinary committed SIGKILL continuity. It is
not a physical-power-loss test, a proof of remote Telegram receipt, or a precise
post-send/pre-mark crash window. Synchronized state/daemon helper tests cover
uncommitted baseline and send-return-before-mark crash windows separately; their
private seams do not exist in the production image. Live Telegram, hosted tunnel
access, arbitrary host filesystems and operational deployment remain unverified
by this offline suite.
