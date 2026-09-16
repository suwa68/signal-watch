# Handoff: Telegram v1 Destination

## Objective

Implement the first outbound notification destination for **SignalWatch**.

Telegram is intentionally implemented early so that SignalWatch has a concrete final output contract before NLP, rule evaluation, and source processing become more complex.

This iteration should prove:

```text
Notification
    |
    v
Telegram Renderer
    |
    v
Telegram Notifier
    |
    v
Telegram Channel
```

The implementation must stay small and must not pull unresolved retry, outbox, routing, or NLP concerns into the Telegram package.

---

## Verified External Facts

The following were re-checked before this handoff was created.

### Go SDK

Use:

```text
github.com/go-telegram/bot
```

Verified baseline at the time of this handoff:

- latest GitHub release: `v1.27.0`
- release date: 2026-09-11
- repository is actively maintained
- README states support for Telegram Bot API 10.3
- package exposes Bot API methods directly, including `SendMessage`
- method shape is based on `context.Context` plus a parameter struct
- package supports custom HTTP clients and custom server URLs
- `bot.New()` performs a `getMe` check by default unless explicitly skipped

Do not hard-code assumptions that require exactly v1.27.0 forever. Let `go.mod` pin the actual resolved dependency version used by the repository.

### Telegram `sendMessage`

Telegram currently documents:

- `chat_id`: integer or string
- `text`: 1–4096 characters after entity parsing
- `parse_mode`: optional
- HTML formatting is supported
- channel posting requires the bot to have the `can_post_messages` administrator right

For HTML mode, external text must be escaped. In particular, raw `<`, `>`, and `&` characters cannot be inserted unescaped.

References:

- https://core.telegram.org/bots/api#sendmessage
- https://core.telegram.org/bots/api#formatting-options
- https://github.com/go-telegram/bot

---

# Decisions for Telegram v1

## 1. Outbound only

SignalWatch v1 only sends messages to Telegram.

Do not implement:

- Telegram polling
- Telegram webhook receiver
- command handlers
- callback handlers
- conversational bot behavior
- incoming Telegram events

The SDK is being used as a Bot API client.

There is no need to start an update-processing loop merely to send outbound messages.

---

## 2. One bot per SignalWatch instance

Version 1 assumes one Telegram bot token per SignalWatch process.

Multiple Telegram destinations/channels are allowed.

Conceptually:

```text
SignalWatch Bot
    |
    +-- destination: stock
    |
    +-- destination: system
    |
    +-- destination: other
```

Do not implement multi-bot routing in this iteration.

---

## 3. Secrets and destination configuration are separate

The bot token is a secret.

Expected environment variable:

```bash
SIGNALWATCH_TELEGRAM_BOT_TOKEN=...
```

Never commit it.

A destination contains a Telegram chat identifier.

Conceptual model:

```go
type Destination struct {
    ID     string
    ChatID string
}
```

`ChatID` must be a string in SignalWatch so it can support both:

- numeric chat/channel IDs such as `-100...`
- Telegram usernames where Telegram accepts `@channelname`

The Telegram task should not design the project's final global configuration system.

Accept destinations through normal Go constructors/function parameters so future config loading can supply them.

---

# Final Notification Contract

The purpose of this iteration is also to stabilize what downstream NLP eventually needs to produce.

Use the following v1 notification model unless the existing repository already contains an equivalent model:

```go
type Notification struct {
    Title       string
    Summary     string
    SourceName  string
    URL         string
    PublishedAt *time.Time
}
```

Semantics:

| Field | Required | Meaning |
|---|---|---|
| `Title` | yes | Human-readable notification title |
| `Summary` | yes | Final concise summary to be delivered |
| `SourceName` | yes | Human-readable source name |
| `URL` | no | Link to the original resource |
| `PublishedAt` | no | Source-provided publication timestamp |

Do not add:

- importance score
- category
- entities
- sentiment
- keywords
- model name
- token usage
- raw NLP response

Those fields may be designed later.

---

# Telegram Message Format

Telegram v1 uses HTML parse mode.

Target presentation:

```text
🔔 <Title>

<Summary>

Source: <SourceName>
Published: <PublishedAt>

View original
```

Actual HTML structure should be equivalent to:

```html
<b>🔔 Murata Announces MLCC Product Changes</b>

Murata has announced adjustments to selected MLCC product lines.

<b>Source:</b> Murata News
<b>Published:</b> 2026-09-15 10:30 CST

<a href="https://example.com/news/123">View original</a>
```

Rules:

1. Escape all external/user/source-derived text.
2. Escape the URL before placing it into an HTML attribute.
3. Omit `Published` entirely when `PublishedAt == nil`.
4. Omit `View original` entirely when `URL == ""`.
5. Do not output empty placeholder lines for omitted fields.
6. Use HTML parse mode.
7. Keep the format deliberately minimal.

The renderer must own Telegram-specific formatting.

`Notification` itself must not contain Telegram HTML.

---

# Message Length

Telegram `sendMessage` allows 1–4096 characters after entity parsing.

Do not allow a long NLP summary to make notification delivery fail.

For v1:

- preserve the complete title/source metadata where reasonable
- truncate the summary when needed
- append a clear ellipsis when truncation occurs
- keep a conservative safety margin below Telegram's hard limit

The implementation may use a conservative internal maximum such as approximately 3800 visible Unicode runes for the complete rendered text.

Do not split one notification into multiple Telegram messages in v1.

This can be reconsidered later.

---

# Link Preview

Disable link previews for v1.

Reason:

> SignalWatch should control the final notification layout rather than allow arbitrary destination websites to expand messages with inconsistent previews.

`go-telegram/bot` exposes Telegram's `LinkPreviewOptions`.

Use the SDK-supported option equivalent to:

```text
is_disabled = true
```

---

# Package Boundary

Recommended structure:

```text
internal/
└── notification/
    ├── model.go
    │
    └── telegram/
        ├── destination.go
        ├── renderer.go
        ├── notifier.go
        └── notifier_test.go
```

Adapt to the existing repository layout when necessary.

Do not create additional abstraction layers merely to match this directory tree.

---

# Renderer

The Telegram renderer converts:

```text
Notification
    |
    v
Telegram HTML string
```

Recommended API:

```go
type Renderer struct {
    // optional renderer settings
}

func (r Renderer) Render(notification notification.Notification) (string, error)
```

Responsibilities:

- validation of fields required specifically for rendering
- HTML escaping
- optional field omission
- timestamp formatting
- length handling/truncation
- deterministic formatting

The renderer must be independently unit-testable.

---

# Notifier

The notifier owns SDK interaction.

Conceptually:

```go
type Notifier struct {
    bot ...
}

func (n *Notifier) Send(
    ctx context.Context,
    destination Destination,
    notification notification.Notification,
) error
```

Internal flow:

```text
Notification
    |
    v
Renderer.Render
    |
    v
bot.SendMessage
```

Telegram-specific SDK types must stay inside the Telegram infrastructure package.

Do not expose `bot.SendMessageParams` to core/domain packages.

---

# SDK Initialization

Production initialization should use the bot token supplied by application assembly/configuration.

The current SDK calls `getMe` during `bot.New()` by default.

Keeping this behavior is acceptable and useful because it validates the bot token during startup.

Do not call `bot.Start()` for the outbound-only implementation.

Tests should not contact Telegram.

Use either:

- a small injected SDK-facing interface/fake inside the Telegram package, or
- the SDK's custom HTTP client/server support

Prefer the simplest implementation that keeps unit tests deterministic.

Do not build a replacement Telegram HTTP client.

---

# Error Handling

Telegram v1 sends once.

Do not implement retry logic inside `TelegramNotifier`.

On failure:

- return/wrap the SDK error
- include useful context such as destination ID
- preserve the underlying error with `%w` where applicable

The SDK already exposes Telegram error behavior including HTTP/API cases such as:

- 400 bad request
- 401 unauthorized
- 403 forbidden
- 429 too many requests

Do not create a large custom error taxonomy in this iteration.

Retry policy belongs to a future delivery layer.

---

# Explicit Non-Goals

Do not implement:

- retry engine
- transactional outbox
- delivery persistence
- notification history
- message editing
- deleting messages
- media/photo/document delivery
- inline keyboards
- Telegram topics
- Telegram commands
- incoming updates
- webhooks
- multiple bots
- Slack/email abstractions
- NLP calls
- rule evaluation
- destination routing rules

The only responsibility is:

```text
given a completed Notification + Telegram Destination
send the correct Telegram message
```

---

# Tests

## Renderer tests

At minimum cover:

- normal notification
- HTML characters in title
- HTML characters in summary
- HTML characters in source name
- URL escaping
- missing URL
- missing PublishedAt
- timestamp rendering
- long summary truncation
- deterministic output
- output remains under the configured Telegram-safe limit

Example hostile/external text:

```text
Murata <MLCC> & "EOL"
```

must not break Telegram HTML.

---

## Notifier tests

At minimum cover:

- correct destination `ChatID`
- correct message text from renderer
- HTML parse mode
- link preview disabled
- context passed through
- SDK error returned/wrapped
- renderer error prevents SDK call

Do not call the real Telegram API from normal unit tests.

---

# Manual Smoke Test

Provide a small, explicit way to manually verify the real destination after unit tests pass.

It may be:

- a small command under `cmd/telegram-smoke`, or
- an existing application command if the repository already has a suitable CLI

The smoke test should require explicit environment variables, for example:

```bash
SIGNALWATCH_TELEGRAM_BOT_TOKEN=...
SIGNALWATCH_TELEGRAM_CHAT_ID=...
```

It should send one clearly identified test notification.

Example:

```text
🔔 SignalWatch Telegram Test

Telegram destination integration is working.

Source: SignalWatch
```

The smoke path must not run as part of `go test ./...`.

Do not embed a token or chat ID in source code.

---

# Telegram Channel Setup Preconditions

Document the operational prerequisites:

1. Create the bot through BotFather.
2. Add the bot to the target channel.
3. Grant it administrator permission allowing channel message posting (`can_post_messages`).
4. Provide the bot token through environment/secrets management.
5. Provide the target chat/channel ID or supported `@username`.

The application is not responsible for provisioning Telegram channels or permissions.

---

# Acceptance Criteria

The task is complete when:

- [ ] Telegram SDK dependency is added.
- [ ] `Notification` v1 model exists.
- [ ] Telegram `Destination` exists.
- [ ] Telegram renderer exists.
- [ ] Renderer produces the agreed HTML format.
- [ ] External text is correctly HTML-escaped.
- [ ] Optional URL is handled.
- [ ] Optional PublishedAt is handled.
- [ ] Long summaries cannot exceed the Telegram-safe rendered limit.
- [ ] Telegram notifier sends using `SendMessage`.
- [ ] HTML parse mode is used.
- [ ] Link previews are disabled.
- [ ] Telegram SDK types do not leak into core packages.
- [ ] No inbound bot processing is started.
- [ ] No retry/outbox implementation is added.
- [ ] Unit tests pass.
- [ ] `go test ./...` passes.
- [ ] Manual smoke path is documented.
- [ ] No Telegram token/chat ID is committed.

---

# Subagent Instructions

Before implementation:

1. inspect the current repository,
2. read `AGENTS.md`,
3. read current architecture/status documents under `docs/`,
4. preserve the existing Go package conventions where reasonable.

If the repository already contains a type or package satisfying one of the contracts above, reuse or minimally extend it rather than creating duplicates.

Do not change unrelated source architecture.

Do not redesign unresolved NLP, item-state, or delivery-reliability semantics.

After implementation, report:

1. files created/modified,
2. dependency/version added,
3. final package/API shape,
4. tests executed and results,
5. manual smoke-test instructions,
6. deviations from this handoff,
7. questions that should return to architecture discussion.
