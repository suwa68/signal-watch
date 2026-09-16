# Telegram Destination v1

Tracking issue: [#3 — Add the first Telegram notification destination](https://github.com/suwa68/signal-watch/issues/3).

## Scope and status

Implemented from [the Telegram handoff](handoffs/telegram-v1-implementation.md):

```text
completed Notification -> Telegram HTML renderer -> SDK SendMessage -> Destination
```

This is an independently callable delivery component. Source collection is not
yet connected to notification production. NLP, rules, routing, persistence,
retry policy, and outbox behavior remain unresolved. The bot receives no updates.

The dependency is `github.com/go-telegram/bot v1.27.0`, pinned in `go.mod` and
verified by `go.sum`. The SDK is the HTTP client implementation; SignalWatch
does not maintain a replacement Bot API client.

## Package and API

```text
internal/notification/model.go
internal/notification/telegram/destination.go
internal/notification/telegram/renderer.go
internal/notification/telegram/notifier.go
cmd/telegram-smoke/main.go
```

Core contract:

```go
type Notification struct {
    Title       string
    Summary     string
    SourceName  string
    URL         string
    PublishedAt *time.Time
}
```

`Title`, `Summary`, and `SourceName` are required plain text. `Summary` is the
final concise delivery summary, with no NLP response envelope. `PublishedAt`
is a source-provided timestamp, not the delivery time.

Telegram API:

```go
type Destination struct {
    ID     string
    ChatID string
}

func New(token string) (*Notifier, error)
func (Renderer) Render(message notification.Notification) (string, error)
func (*Notifier) Send(ctx context.Context, destination Destination, message notification.Notification) error
```

Application assembly supplies the token to `New` once per process and reuses
that notifier for any number of destinations. The package does not read the
environment or define the project's global configuration system. The smoke
command is the current assembly example. Both destination fields must be
nonblank. `ChatID` accepts a numeric ID as a string or a supported `@username`.

Only the Telegram infrastructure package imports SDK types. Its one-method
SDK-facing interface and SDK options are private testing seams.

## Rendering policies

The zero-value `Renderer{}` is ready to use and has no mutable state. It produces:

```html
<b>🔔 Murata Announces MLCC Product Changes</b>

Murata has announced adjustments to selected MLCC product lines.

<b>Source:</b> Murata News
<b>Published:</b> 2026-09-15 10:30 CST

<a href="https://example.com/news/123">View original</a>
```

- All external text and link attributes are HTML-escaped. Required text must
  contain non-whitespace content and valid UTF-8; otherwise rendering fails.
- An empty URL omits the link. A supplied URL must be an absolute HTTP/HTTPS URL
  with a host and without embedded credentials. The renderer does not fetch or
  canonicalize the URL.
- A nil `PublishedAt` omits the entire timestamp line. Timestamps use the input
  `time.Time` location and layout `2006-01-02 15:04 MST`. There is no implicit
  conversion to the process's timezone.
- Omitted fields introduce no placeholder lines.
- The complete visible text has a fixed budget of **3800 UTF-16 code units**,
  including labels, line breaks, link label, and ellipsis. Markup and the link
  attribute do not count as visible text. This is more conservative than the
  handoff's suggested rune count: supplementary characters such as many emoji
  consume two units.
- Title and source metadata are preserved. Only the summary is shortened, with
  a trailing `…`. Truncation happens before HTML escaping, at Unicode code point
  boundaries, so it does not cut UTF-8 or HTML entities. It does not promise to
  preserve combined graphemes such as an entire multi-code-point emoji sequence.
- If metadata leaves no room for even an ellipsis, rendering returns an error
  before an SDK call. One notification is never split into multiple messages.

The fixed limit, explicit oversized-metadata error, UTF-8/URL validation, and
input timezone policy are implementation choices within the handoff's renderer
scope. They do not define source canonicalization or future delivery policies.

## SDK lifecycle and errors

`New` retains the SDK's startup `getMe` validation and default five-second
initialization timeout. It never calls `Start` or `StartWebhook`.

`Send` passes the caller's context to one `SendMessage` call, selects HTML parse
mode, and sets `LinkPreviewOptions.IsDisabled` to true. Cancellation and deadlines
belong to the caller. An already-canceled context or invalid rendering/destination
input prevents the SDK call.

Errors include destination ID and preserve SDK errors using `%w`. Callers can
use `errors.Is`/`errors.As`, including for Telegram's rate-limit error and its
retry-after value. No retry takes place here. The smoke command exits nonzero on
configuration, initialization, or send failure. It gives delivery a 20-second
context deadline and handles interrupt/termination signals; SDK startup retains
its separate timeout.

## Channel setup and manual smoke test

1. Create a bot through Telegram's BotFather.
2. Add the bot to the target channel.
3. Make it an administrator with `can_post_messages` permission.
4. Supply the token through environment/secrets management as
   `SIGNALWATCH_TELEGRAM_BOT_TOKEN`.
5. Supply a numeric channel/chat ID or supported `@username` as
   `SIGNALWATCH_TELEGRAM_CHAT_ID`.

Keep real tokens and destination identifiers out of committed source/config.
Channel provisioning and permissions are managed in Telegram.

### Local settings file

Create `.env.telegram.local` in the project root and fill in both values:

```dotenv
SIGNALWATCH_TELEGRAM_BOT_TOKEN=
SIGNALWATCH_TELEGRAM_CHAT_ID=
```

The token comes from BotFather. Chat ID is the supported `@username` or numeric
ID of the target channel. `.gitignore` excludes this local file; each developer
creates their own copy. On macOS/Linux, `chmod 600 .env.telegram.local` limits
file access to the owner.

With Docker Compose supporting `--env-from-file` (available in the current
development environment), explicitly load this file and send one notification:

```sh
docker compose run --rm \
  --env-from-file .env.telegram.local \
  go go run ./cmd/telegram-smoke
```

The application itself does not read dotenv files. Docker Compose supplies these
values to the smoke command as environment variables.

### Exported environment variables

Alternatively, after the two variables are exported in your shell, explicitly run:

```sh
docker compose run --rm \
  -e SIGNALWATCH_TELEGRAM_BOT_TOKEN \
  -e SIGNALWATCH_TELEGRAM_CHAT_ID \
  go go run ./cmd/telegram-smoke
```

With a local Go 1.25+ installation, the equivalent is:

```sh
go run ./cmd/telegram-smoke
```

This performs startup validation and sends one message:

```text
🔔 SignalWatch Telegram Test

Telegram destination integration is working.

Source: SignalWatch
```

Success prints `SignalWatch Telegram test notification sent.` Missing variables
fail before SDK initialization. This command is never invoked by normal tests.
A real-channel smoke test remains a manual operational check. On 2026-09-16,
the command was run with `.env.telegram.local` against the configured private
channel: startup validation and one test-message send succeeded, and the command
exited with status 0. This confirms Telegram API acceptance; visual receipt in
the Telegram client has not yet been confirmed by the user. Credentials and the
real channel ID are kept out of this record.

## Automated validation

```sh
docker compose run --rm go go test ./...
docker compose run --rm go go test -race ./...
docker compose run --rm go go vet ./...
```

Tests cover exact formatting, optional fields, escaping and link round-tripping,
timestamp location, ASCII/CJK/emoji length boundaries, ellipsis behavior,
oversized metadata, invalid input, destination and context propagation, SDK
multipart serialization, startup validation, cancellation during a request,
and Telegram API errors (400, 401, 403, 429) without retries. Fakes and local
`httptest.Server` instances keep tests independent of live Telegram and secrets.

## References

- [Telegram sendMessage](https://core.telegram.org/bots/api#sendmessage)
- [Telegram formatting options](https://core.telegram.org/bots/api#formatting-options)
- [Telegram LinkPreviewOptions](https://core.telegram.org/bots/api#linkpreviewoptions)
- [Telegram channel administrator rights](https://core.telegram.org/bots/api#chatmemberadministrator)
- [go-telegram/bot v1.27.0](https://github.com/go-telegram/bot/tree/v1.27.0)
