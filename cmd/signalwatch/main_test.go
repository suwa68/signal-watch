package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/notification/telegram"
	monitor "github.com/suwa68/signal-watch/internal/runtime"
	"github.com/suwa68/signal-watch/internal/state"
)

const syntheticToken = "synthetic-secret-token"

func env(key string) string {
	if key == "SIGNALWATCH_TELEGRAM_BOT_TOKEN" {
		return syntheticToken
	}
	if key == "SIGNALWATCH_TELEGRAM_CHAT_ID" {
		return "123"
	}
	return ""
}
func definitions(t *testing.T, id, interval, url string) string {
	t.Helper()
	dir := t.TempDir()
	writeDefinition(t, dir, "one.yaml", id, interval, url)
	return dir
}
func writeDefinition(t *testing.T, dir, name, id, interval, url string) {
	t.Helper()
	content := fmt.Sprintf("version: 1\nid: %s\nname: News\ntype: html\ninterval_seconds: %s\nconfig:\n  url: %s\n  selectors:\n    item: article\n    title: h2\n    url: a\n", id, interval, url)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

type testStore struct {
	state.StateStore
	close func() error
}

func (s *testStore) Close() error { return s.close() }

type senderFunc func(context.Context, telegram.Destination, notification.Notification) error

func (f senderFunc) Send(c context.Context, d telegram.Destination, n notification.Notification) error {
	return f(c, d, n)
}
func testDependencies(ctx context.Context) dependencies {
	return dependencies{signalContext: func() (context.Context, context.CancelFunc) { return ctx, func() {} }, openStore: func(context.Context, string) (ownedStore, error) {
		return &testStore{state.NewMemoryStateStore(), func() error { return nil }}, nil
	}, newNotifier: func(string) (notifier, error) {
		return senderFunc(func(context.Context, telegram.Destination, notification.Notification) error { return nil }), nil
	}}
}
func TestRejectsConfigurationBeforeStateOrTelegram(t *testing.T) {
	valid := definitions(t, "news", "1", "http://127.0.0.1/")
	duplicate := definitions(t, "news", "1", "http://127.0.0.1/")
	writeDefinition(t, duplicate, "two.yaml", "news", "1", "http://127.0.0.1/")
	invalid := definitions(t, "news", "1", "file:///tmp/news")
	overflow := definitions(t, "news", fmt.Sprint(math.MaxInt64/int64(time.Second)+1), "http://127.0.0.1/")
	for _, tc := range []struct {
		name   string
		args   []string
		getenv func(string) string
	}{
		{"missing sources", nil, env}, {"unknown", []string{"--wat"}, env}, {"position", []string{"--sources-dir", valid, "unexpected"}, env},
		{"state empty", []string{"--sources-dir", valid, "--state-dir", " "}, env}, {"env", []string{"--sources-dir", valid}, func(string) string { return " " }},
		{"empty catalog", []string{"--sources-dir", t.TempDir()}, env}, {"duplicate", []string{"--sources-dir", duplicate}, env}, {"invalid settings", []string{"--sources-dir", invalid}, env}, {"overflow", []string{"--sources-dir", overflow}, env},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDependencies(context.Background())
			d.openStore = func(context.Context, string) (ownedStore, error) {
				t.Fatal("state acquired before validation")
				return nil, nil
			}
			d.newNotifier = func(string) (notifier, error) { t.Fatal("Telegram before validation"); return nil, nil }
			var out bytes.Buffer
			if code := run(tc.args, tc.getenv, &out, &out, d); code != 2 {
				t.Fatalf("exit %d output %s", code, &out)
			}
		})
	}
}
func TestStartupOrderAndCancellationAfterNotifier(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := testDependencies(ctx)
	var order []string
	d.openStore = func(_ context.Context, dir string) (ownedStore, error) {
		if dir != "/state" {
			t.Errorf("default state dir %q", dir)
		}
		order = append(order, "state")
		return &testStore{state.NewMemoryStateStore(), func() error { order = append(order, "close"); return nil }}, nil
	}
	d.newNotifier = func(token string) (notifier, error) {
		if token != syntheticToken {
			t.Error("token lost")
		}
		order = append(order, "notifier")
		cancel()
		return senderFunc(func(context.Context, telegram.Destination, notification.Notification) error { return nil }), nil
	}
	d.schedule = func(context.Context, []monitor.ScheduledSource, func(config.SourceConfig, error)) error {
		t.Error("scheduled after startup cancellation")
		return nil
	}
	if got := run([]string{"--sources-dir", definitions(t, "news", "1", "http://127.0.0.1/")}, env, io.Discard, io.Discard, d); got != 0 {
		t.Fatalf("exit %d", got)
	}
	if strings.Join(order, ",") != "state,notifier,close" {
		t.Fatalf("order %v", order)
	}
}
func TestOwnershipFailurePreventsNotifier(t *testing.T) {
	d := testDependencies(context.Background())
	d.openStore = func(context.Context, string) (ownedStore, error) { return nil, errors.New("owned") }
	d.newNotifier = func(string) (notifier, error) { t.Fatal("notifier started despite ownership failure"); return nil, nil }
	if got := run([]string{"--sources-dir", definitions(t, "news", "1", "http://127.0.0.1/")}, env, io.Discard, io.Discard, d); got != 1 {
		t.Fatalf("exit %d", got)
	}
}
func TestShutdownWaitsForWorkerAndPreservesFatalAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name                string
		workerErr, closeErr error
		want                int
	}{{"signal", context.Canceled, nil, 0}, {"fatal plus signal", errors.Join(context.Canceled, state.ErrFatal), nil, 1}, {"cleanup", context.Canceled, errors.New("close failed"), 1}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d := testDependencies(ctx)
			started := make(chan struct{})
			release := make(chan struct{})
			finished := make(chan struct{})
			closed := make(chan struct{})
			d.openStore = func(context.Context, string) (ownedStore, error) {
				return &testStore{state.NewMemoryStateStore(), func() error {
					select {
					case <-finished:
					default:
						t.Error("store closed before worker finished")
					}
					close(closed)
					return tc.closeErr
				}}, nil
			}
			d.schedule = func(ctx context.Context, _ []monitor.ScheduledSource, _ func(config.SourceConfig, error)) error {
				close(started)
				<-ctx.Done()
				<-release
				close(finished)
				return tc.workerErr
			}
			result := make(chan int, 1)
			go func() {
				result <- run([]string{"--sources-dir", definitions(t, "news", "1", "http://127.0.0.1/")}, env, io.Discard, io.Discard, d)
			}()
			<-started
			cancel()
			select {
			case <-closed:
				t.Fatal("closed while worker blocked")
			default:
			}
			close(release)
			if got := <-result; got != tc.want {
				t.Fatalf("exit %d want %d", got, tc.want)
			}
		})
	}
}
func TestNoncooperativeShutdownDoesNotCloseActiveStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := testDependencies(ctx)
	d.shutdownGrace = 20 * time.Millisecond
	started := make(chan struct{})
	release := make(chan struct{})
	closed := make(chan struct{})
	d.openStore = func(context.Context, string) (ownedStore, error) {
		return &testStore{state.NewMemoryStateStore(), func() error { close(closed); return nil }}, nil
	}
	d.schedule = func(context.Context, []monitor.ScheduledSource, func(config.SourceConfig, error)) error {
		close(started)
		<-release
		return context.Canceled
	}
	result := make(chan int, 1)
	go func() {
		result <- run([]string{"--sources-dir", definitions(t, "news", "1", "http://127.0.0.1/")}, env, io.Discard, io.Discard, d)
	}()
	<-started
	cancel()
	if got := <-result; got != 1 {
		t.Fatalf("timeout exit %d", got)
	}
	select {
	case <-closed:
		t.Fatal("active store closed at timeout")
	default:
	}
	close(release)
	<-closed
}
func TestOperationalErrorsAreBoundedAndRedacted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := testDependencies(ctx)
	d.schedule = func(_ context.Context, s []monitor.ScheduledSource, report func(config.SourceConfig, error)) error {
		report(s[0].Config, fmt.Errorf("request https://api.telegram.org/bot%s/sendMessage: %s %s", syntheticToken, syntheticToken, strings.Repeat("x", 5000)))
		cancel()
		return context.Canceled
	}
	var out bytes.Buffer
	run([]string{"--sources-dir", definitions(t, "news", "1", "http://127.0.0.1/")}, env, &out, &out, d)
	if strings.Contains(out.String(), syntheticToken) || strings.Contains(out.String(), "api.telegram.org") || !strings.Contains(out.String(), "source_run_completed") || !strings.Contains(out.String(), `"source_id":"news"`) {
		t.Fatalf("unsafe/missing operational log: %.300s", &out)
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if len(line) > 4096 {
			t.Fatalf("unbounded log line %d", len(line))
		}
	}
}

// Real scheduler/request cancellation: no live website or Telegram endpoint.
func TestSignalCancelsInFlightRequest(t *testing.T) {
	entered := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(canceled) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan int, 1)
	dir := definitions(t, "news", "1", server.URL)
	go func() {
		result <- run([]string{"--sources-dir", dir}, env, io.Discard, io.Discard, testDependencies(ctx))
	}()
	<-entered
	cancel()
	if got := <-result; got != 0 {
		t.Fatalf("exit %d", got)
	}
	<-canceled
}

// Buffers used by concurrent source-report callbacks must also serialize writes.
type lockedBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}

type blockingWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w blockingWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}
func TestBlockedOperationalLogCannotPreventShutdownTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := testDependencies(ctx)
	d.shutdownGrace = 20 * time.Millisecond
	entered := make(chan struct{})
	release := make(chan struct{})
	closed := make(chan struct{})
	d.openStore = func(context.Context, string) (ownedStore, error) {
		return &testStore{state.NewMemoryStateStore(), func() error { close(closed); return nil }}, nil
	}
	dir := definitions(t, "news", "1", "http://127.0.0.1/")
	result := make(chan int, 1)
	go func() {
		result <- run([]string{"--sources-dir", dir}, env, blockingWriter{entered, release}, io.Discard, d)
	}()
	<-entered
	cancel()
	select {
	case code := <-result:
		if code != 1 {
			t.Errorf("exit %d", code)
		}
		close(release)
	case <-time.After(250 * time.Millisecond):
		close(release)
		<-result
		t.Fatal("blocked operational log prevented bounded shutdown")
	}
	<-closed
}
