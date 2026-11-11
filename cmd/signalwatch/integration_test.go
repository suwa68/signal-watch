package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/notification/telegram"
	"github.com/suwa68/signal-watch/internal/query"
	monitor "github.com/suwa68/signal-watch/internal/runtime"
	"github.com/suwa68/signal-watch/internal/state"
)

func articles(titles ...string) string {
	var b strings.Builder
	for _, title := range titles {
		fmt.Fprintf(&b, "<article><h2>%s</h2><a href=\"/%s\">link</a></article>", title, title)
	}
	return b.String()
}

type contextStore struct {
	*state.SQLiteStateStore
	t     *testing.T
	marks int
}

func (s *contextStore) MarkItemSeen(ctx context.Context, id, key string) error {
	if _, ok := ctx.Deadline(); ok {
		s.t.Error("mark inherited send deadline")
	}
	if ctx.Err() != nil {
		s.t.Error("send cancellation leaked into mark")
	}
	s.marks++
	return s.SQLiteStateStore.MarkItemSeen(ctx, id, key)
}
func TestRealSQLiteBaselineReopenAndQueryCoexistence(t *testing.T) {
	var body atomic.Value
	body.Store(articles("A", "B"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body.Load().(string)) }))
	defer server.Close()
	dir := definitions(t, "news", "1", server.URL)
	stateDir := t.TempDir()
	args := []string{"--sources-dir", dir, "--state-dir", stateDir}
	var sent []string
	var sendContext context.Context
	var storage *contextStore
	d := testDependencies(context.Background())
	d.openStore = func(ctx context.Context, dir string) (ownedStore, error) {
		s, err := state.OpenSQLite(ctx, dir)
		storage = &contextStore{s, t, 0}
		return storage, err
	}
	d.newNotifier = func(string) (notifier, error) {
		return senderFunc(func(ctx context.Context, destination telegram.Destination, message notification.Notification) error {
			if destination.ID != "monitor" || destination.ChatID != "123" {
				t.Errorf("destination %+v", destination)
			}
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 20*time.Second || time.Until(deadline) < 19*time.Second {
				t.Errorf("send deadline %v %v", deadline, ok)
			}
			rendered, err := (telegram.Renderer{}).Render(message)
			if err != nil {
				return err
			}
			sent = append(sent, rendered)
			sendContext = ctx
			return nil
		}), nil
	}
	d.schedule = func(ctx context.Context, s []monitor.ScheduledSource, _ func(config.SourceConfig, error)) error {
		if err := s[0].Runner.RunOnce(ctx, s[0].Config); err != nil {
			return err
		}
		if len(sent) != 0 {
			t.Error("baseline sent")
		}
		body.Store(articles("A", "B", "C"))
		catalog, service, err := query.LoadHTMLCatalog(ctx, dir, nil)
		if err != nil {
			return err
		}
		definition, err := catalog.Lookup("news")
		if err != nil {
			return err
		}
		snapshot, err := service.Collect(ctx, definition, query.DefaultLimits())
		if err != nil {
			return err
		}
		if len(snapshot.Items) != 3 {
			t.Errorf("query items %d", len(snapshot.Items))
		}
		return s[0].Runner.RunOnce(ctx, s[0].Config)
	}
	if got := run(args, env, io.Discard, io.Discard, d); got != 0 {
		t.Fatalf("exit %d", got)
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "C") {
		t.Fatalf("rendered sends %v", sent)
	}
	if storage.marks != 1 {
		t.Fatalf("marks %d", storage.marks)
	}
	if sendContext.Err() != context.Canceled {
		t.Fatal("send child not canceled after return")
	}
	d.schedule = func(ctx context.Context, s []monitor.ScheduledSource, _ func(config.SourceConfig, error)) error {
		return s[0].Runner.RunOnce(ctx, s[0].Config)
	}
	if got := run(args, env, io.Discard, io.Discard, d); got != 0 {
		t.Fatalf("reopen exit %d", got)
	}
	if len(sent) != 1 {
		t.Fatalf("reopen resent C: %v", sent)
	}
}

func TestHelpExitsWithoutStartup(t *testing.T) {
	d := testDependencies(context.Background())
	d.openStore = func(context.Context, string) (ownedStore, error) { t.Fatal("help acquired state"); return nil, nil }
	if got := run([]string{"--help"}, env, io.Discard, io.Discard, d); got != 0 {
		t.Fatalf("help exit %d", got)
	}
}
func TestJSONLogsBoundEscapedInput(t *testing.T) {
	var out lockedBuffer
	logs := newOperationalLog(&out, &out, syntheticToken)
	logs.event("source_run_completed", strings.Repeat("\x00", 5000), fmt.Errorf("%s", strings.Repeat("\x00", 5000)))
	if out.Len() > 4096 {
		t.Fatalf("escaped log bytes %d", out.Len())
	}
}

func TestActualStateOwnerPreventsTelegramInitialization(t *testing.T) {
	stateDir := t.TempDir()
	store, err := state.OpenSQLite(context.Background(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	d := testDependencies(context.Background())
	d.openStore = nil
	d.newNotifier = func(string) (notifier, error) { t.Fatal("getMe attempted before ownership"); return nil, nil }
	if got := run([]string{"--sources-dir", definitions(t, "news", "1", "http://127.0.0.1/"), "--state-dir", stateDir}, env, io.Discard, io.Discard, d); got != 1 {
		t.Fatalf("second owner exit %d", got)
	}
}

func TestFlagDiagnosticsRedactTokenAndTelegramURL(t *testing.T) {
	for _, arg := range []string{"--" + syntheticToken, "--https://api.telegram.org/botother-token/getMe"} {
		var out strings.Builder
		if got := run([]string{arg}, env, io.Discard, &out, testDependencies(context.Background())); got != 2 {
			t.Fatalf("exit %d", got)
		}
		if strings.Contains(out.String(), syntheticToken) || strings.Contains(out.String(), "api.telegram.org") {
			t.Fatalf("unsafe flag output: %s", out.String())
		}
	}
}
