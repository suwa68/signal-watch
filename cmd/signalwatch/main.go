// Command signalwatch monitors configured HTML sources with durable seen state.
package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/notification/telegram"
	monitor "github.com/suwa68/signal-watch/internal/runtime"
	"github.com/suwa68/signal-watch/internal/state"
)

const shutdownGrace = 10 * time.Second
const sendTimeout = 20 * time.Second

type ownedStore interface {
	state.StateStore
	Close() error
}
type notifier interface {
	Send(context.Context, telegram.Destination, notification.Notification) error
}

// Private infrastructure seams keep command tests offline without runtime flags.
type dependencies struct {
	signalContext func() (context.Context, context.CancelFunc)
	openStore     func(context.Context, string) (ownedStore, error)
	newNotifier   func(string) (notifier, error)
	schedule      func(context.Context, []monitor.ScheduledSource, func(config.SourceConfig, error)) error
	shutdownGrace time.Duration
}

func (d dependencies) defaults() dependencies {
	if d.signalContext == nil {
		d.signalContext = func() (context.Context, context.CancelFunc) {
			return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		}
	}
	if d.openStore == nil {
		d.openStore = func(ctx context.Context, dir string) (ownedStore, error) { return state.OpenSQLite(ctx, dir) }
	}
	if d.newNotifier == nil {
		d.newNotifier = func(token string) (notifier, error) { return telegram.New(token) }
	}
	if d.schedule == nil {
		d.schedule = monitor.Run
	}
	if d.shutdownGrace == 0 {
		d.shutdownGrace = shutdownGrace
	}
	return d
}
func main() { os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr, dependencies{})) }

// run supervises startup as well as workers: a signal during bounded SDK getMe
// starts the same shutdown deadline. On timeout main exits without closing a
// store that a noncooperative worker might still use.
func run(args []string, getenv func(string) string, stdout, stderr io.Writer, d dependencies) int {
	logs := newOperationalLog(stdout, stderr, getenv("SIGNALWATCH_TELEGRAM_BOT_TOKEN"))
	var diagnostics bytes.Buffer
	options, code := parseOptions(args, getenv, &diagnostics)
	if diagnostics.Len() > 0 {
		_, _ = io.WriteString(stderr, logs.safe(diagnostics.String(), 2048))
	}
	if code != 0 || options.help {
		return code
	}
	d = d.defaults()
	ctx, stop := d.signalContext()
	defer stop()
	result := make(chan outcome, 1)
	go func() { result <- execute(ctx, options, d, logs) }()
	select {
	case done := <-result:
		return finish(done, logs)
	case <-ctx.Done():
		timer := time.NewTimer(d.shutdownGrace)
		defer timer.Stop()
		select {
		case done := <-result:
			return finish(done, logs)
		case <-timer.C:
			logs.finalFailure("shutdown grace exceeded")
			return 1
		}
	}
}
