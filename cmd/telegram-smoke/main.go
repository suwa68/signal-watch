// Command telegram-smoke sends one explicit test notification to Telegram.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/notification/telegram"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("SignalWatch Telegram test notification sent.")
}

func run() error {
	token := os.Getenv("SIGNALWATCH_TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("SIGNALWATCH_TELEGRAM_CHAT_ID")
	if strings.TrimSpace(token) == "" || strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("set SIGNALWATCH_TELEGRAM_BOT_TOKEN and SIGNALWATCH_TELEGRAM_CHAT_ID to send one test notification")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	notifier, err := telegram.New(token)
	if err != nil {
		return err
	}
	return notifier.Send(ctx, telegram.Destination{ID: "smoke-test", ChatID: chatID}, notification.Notification{
		Title:      "SignalWatch Telegram Test",
		Summary:    "Telegram destination integration is working.",
		SourceName: "SignalWatch",
	})
}
