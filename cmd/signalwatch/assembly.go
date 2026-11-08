package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/suwa68/signal-watch/internal/application"
	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/notification/telegram"
	"github.com/suwa68/signal-watch/internal/query"
	monitor "github.com/suwa68/signal-watch/internal/runtime"
)

type options struct {
	sourcesDir, stateDir, token, chatID string
	help                                bool
}
type outcome struct {
	code int
	err  error
}

func parseOptions(args []string, getenv func(string) string, stderr io.Writer) (options, int) {
	var o options
	flags := flag.NewFlagSet("signalwatch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&o.sourcesDir, "sources-dir", "", "directory containing source YAML definitions")
	flags.StringVar(&o.stateDir, "state-dir", "/state", "directory for durable monitoring state")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			o.help = true
			return o, 0
		}
		return o, 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return o, 2
	}
	if strings.TrimSpace(o.sourcesDir) == "" || strings.TrimSpace(o.stateDir) == "" {
		fmt.Fprintln(stderr, "--sources-dir and --state-dir must be nonempty")
		return o, 2
	}
	o.token = getenv("SIGNALWATCH_TELEGRAM_BOT_TOKEN")
	o.chatID = getenv("SIGNALWATCH_TELEGRAM_CHAT_ID")
	if strings.TrimSpace(o.token) == "" || strings.TrimSpace(o.chatID) == "" {
		fmt.Fprintln(stderr, "set SIGNALWATCH_TELEGRAM_BOT_TOKEN and SIGNALWATCH_TELEGRAM_CHAT_ID")
		return o, 2
	}
	return o, 0
}
func execute(ctx context.Context, o options, d dependencies, logs *operationalLog) (result outcome) {
	catalog, collector, err := query.LoadHTMLCatalogAndRunner(ctx, o.sourcesDir, nil)
	if err != nil {
		return canceledOrError(ctx, 2, fmt.Errorf("load source catalog: %w", err))
	}
	definitions := catalog.Definitions()
	if len(definitions) == 0 {
		return outcome{2, errors.New("at least one source is required")}
	}
	for _, definition := range definitions {
		seconds := int64(definition.Config.IntervalSeconds)
		if seconds <= 0 || seconds > math.MaxInt64/int64(time.Second) {
			return outcome{2, fmt.Errorf("source %q: interval_seconds must be positive and fit time.Duration", definition.Config.ID)}
		}
	}
	if err := ctx.Err(); err != nil {
		return outcome{0, nil}
	}
	store, err := d.openStore(ctx, o.stateDir)
	if err != nil {
		return canceledOrError(ctx, 1, fmt.Errorf("open monitoring state: %w", err))
	}
	defer func() {
		if err := store.Close(); err != nil {
			result = outcome{1, errors.Join(result.err, fmt.Errorf("close monitoring state: %w", err))}
		}
	}()
	if ctx.Err() != nil {
		return outcome{0, nil}
	}
	sender, err := d.newNotifier(o.token)
	// SDK getMe has its existing five-second bound and no caller context API.
	// Inspect cancellation immediately afterward, preserving initialization errors.
	if err != nil {
		return canceledOrError(ctx, 1, err)
	}
	if ctx.Err() != nil {
		return outcome{0, nil}
	}
	destination := telegram.Destination{ID: "monitor", ChatID: o.chatID}
	send := application.NotificationSenderFunc(func(ctx context.Context, n notification.Notification) error {
		child, cancel := context.WithTimeout(ctx, sendTimeout)
		defer cancel()
		return sender.Send(child, destination, n)
	})
	scheduled := make([]monitor.ScheduledSource, 0, len(definitions))
	for _, definition := range definitions {
		pipeline, err := application.NewPipeline(collector, store, send)
		if err != nil {
			return outcome{1, err}
		}
		scheduled = append(scheduled, monitor.ScheduledSource{Config: definition.Config, Runner: pipeline})
	}
	if ctx.Err() != nil {
		return outcome{0, nil}
	}
	for _, s := range scheduled {
		logs.event("startup", s.Config.ID, nil)
	}
	err = d.schedule(ctx, scheduled, func(cfg config.SourceConfig, err error) { logs.event("source_run_completed", cfg.ID, err) })
	return canceledOrError(ctx, 1, err)
}
func canceledOrError(ctx context.Context, code int, err error) outcome {
	if err == nil || (ctx.Err() != nil && onlyCanceled(err)) {
		return outcome{0, nil}
	}
	return outcome{code, err}
}

// errors.Is alone would swallow a fatal/cleanup cause joined with cancellation.
func onlyCanceled(err error) bool {
	if err == context.Canceled {
		return true
	}
	if many, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range many.Unwrap() {
			if !onlyCanceled(child) {
				return false
			}
		}
		return true
	}
	if child := errors.Unwrap(err); child != nil {
		return onlyCanceled(child)
	}
	return false
}
func finish(done outcome, logs *operationalLog) int {
	if done.err != nil {
		logs.finalFailure(done.err.Error())
	}
	return done.code
}
