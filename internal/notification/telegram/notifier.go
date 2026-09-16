package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/suwa68/signal-watch/internal/notification"
)

// messageSender keeps the SDK seam private to this infrastructure package.
type messageSender interface {
	SendMessage(context.Context, *bot.SendMessageParams) (*models.Message, error)
}

// Notifier uses one bot for any number of destinations. Construct one per
// process with New and reuse it. It has no update loop or retry policy.
type Notifier struct {
	client   messageSender
	renderer Renderer
}

// New initializes the SDK using the application-supplied secret token. The SDK
// validates it through getMe at startup (with its default five-second timeout).
// It does not start inbound update processing.
func New(token string) (*Notifier, error) {
	return newNotifier(token)
}

// Options are private so SDK configuration types stay inside this package.
func newNotifier(token string, options ...bot.Option) (*Notifier, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("Telegram bot token is required")
	}
	client, err := bot.New(token, options...)
	if err != nil {
		return nil, fmt.Errorf("initialize Telegram bot: %w", err)
	}
	return &Notifier{client: client}, nil
}

// Send renders and sends exactly once. The caller controls cancellation and
// deadlines. SDK errors are wrapped with destination context for the caller.
func (n *Notifier) Send(ctx context.Context, destination Destination, message notification.Notification) error {
	if n == nil || n.client == nil {
		return fmt.Errorf("Telegram notifier is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("send Telegram destination %q: %w", destination.ID, err)
	}
	if strings.TrimSpace(destination.ID) == "" {
		return fmt.Errorf("Telegram destination ID is required")
	}
	if strings.TrimSpace(destination.ChatID) == "" {
		return fmt.Errorf("Telegram destination %q: chat ID is required", destination.ID)
	}
	text, err := n.renderer.Render(message)
	if err != nil {
		return fmt.Errorf("render Telegram destination %q: %w", destination.ID, err)
	}
	_, err = n.client.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    destination.ChatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: bot.True(),
		},
	})
	if err != nil {
		return fmt.Errorf("send Telegram destination %q: %w", destination.ID, err)
	}
	return nil
}
