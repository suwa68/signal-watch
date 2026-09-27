package application_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/suwa68/signal-watch/internal/application"
	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/itemkey"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/notification/telegram"
	"github.com/suwa68/signal-watch/internal/source"
	htmlsource "github.com/suwa68/signal-watch/internal/source/html"
	"github.com/suwa68/signal-watch/internal/state"
)

func TestPipelineFromYAMLToNotifications(t *testing.T) {
	t.Parallel()

	const baselineHTML = `
<article class="news-item">
  <a class="title" href="/releases/a">A</a>
</article>
<article class="news-item">
  <a class="title" href="/releases/b">B</a>
</article>`
	const newItemHTML = `
<article class="news-item">
  <a class="title" href="/releases/c?edition=1&amp;lang=en">C &lt;release&gt; &amp; update</a>
  <p class="content">Original content must not become a summary.</p>
</article>`
	var page atomic.Value
	page.Store(baselineHTML)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/news" {
			http.NotFound(writer, request)
			return
		}
		requests.Add(1)
		_, _ = io.WriteString(writer, page.Load().(string))
	}))
	t.Cleanup(server.Close)

	directory := t.TempDir()
	definition := fmt.Sprintf(`version: 1
id: example_news
name: "Example & News"
type: html
interval_seconds: 300
config:
  url: %q
  selectors:
    item: article.news-item
    title: .title
    url: .title
    content: .content
`, server.URL+"/news")
	if err := os.WriteFile(filepath.Join(directory, "example.yaml"), []byte(definition), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	ctx := context.Background()
	documents, err := config.NewFileSourceDefinitionRepository(directory).List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(documents) != 1 {
		t.Fatalf("List() returned %d documents, want 1", len(documents))
	}
	decoded, err := config.NewYAMLConfigDecoder().Decode(documents[0])
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	sourceConfig, err := config.ParseSourceConfig(decoded)
	if err != nil {
		t.Fatalf("ParseSourceConfig() error = %v", err)
	}
	registry := source.NewSourceAdapterRegistry()
	if err := registry.Register("html", htmlsource.New(server.Client())); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	items := []source.MonitorItem{
		{SourceID: sourceConfig.ID, Title: "A", URL: server.URL + "/releases/a"},
		{SourceID: sourceConfig.ID, Title: "B", URL: server.URL + "/releases/b"},
		{
			SourceID: sourceConfig.ID,
			Title:    "C <release> & update",
			URL:      server.URL + "/releases/c?edition=1&lang=en",
			Content:  "Original content must not become a summary.",
		},
	}
	keys := make([]string, len(items))
	for i, item := range items {
		keys[i], err = itemkey.Build(item)
		if err != nil {
			t.Fatalf("Build(%q) error = %v", item.Title, err)
		}
	}
	store := state.NewMemoryStateStore()
	assertSeen := func(index int, want bool) {
		t.Helper()
		seen, err := store.HasItem(ctx, sourceConfig.ID, keys[index])
		if err != nil || seen != want {
			t.Fatalf("HasItem(%q) = %v, %v, want %v, nil", items[index].Title, seen, err, want)
		}
	}
	var sent []notification.Notification
	var rendered []string
	sender := application.NotificationSenderFunc(func(ctx context.Context, message notification.Notification) error {
		// A normal item must remain unseen until rendering and delivery succeed.
		assertSeen(2, false)
		text, err := (telegram.Renderer{}).Render(message)
		if err != nil {
			return err
		}
		sent = append(sent, message)
		rendered = append(rendered, text)
		return nil
	})
	pipeline, err := application.NewPipeline(source.NewSourceRunner(registry), store, sender)
	if err != nil {
		t.Fatalf("NewPipeline() error = %v", err)
	}

	if err := pipeline.RunOnce(ctx, sourceConfig); err != nil {
		t.Fatalf("baseline RunOnce() error = %v", err)
	}
	initialized, err := store.IsSourceInitialized(ctx, sourceConfig.ID)
	if err != nil || !initialized {
		t.Fatalf("IsSourceInitialized() = %v, %v, want true, nil", initialized, err)
	}
	if len(sent) != 0 {
		t.Fatalf("baseline sent %d notifications, want 0", len(sent))
	}
	assertSeen(0, true)
	assertSeen(1, true)
	assertSeen(2, false)

	page.Store(baselineHTML + newItemHTML)
	if err := pipeline.RunOnce(ctx, sourceConfig); err != nil {
		t.Fatalf("new-item RunOnce() error = %v", err)
	}
	wantSent := []notification.Notification{{
		Title:      "C <release> & update",
		SourceName: "Example & News",
		URL:        server.URL + "/releases/c?edition=1&lang=en",
	}}
	if !reflect.DeepEqual(sent, wantSent) {
		t.Fatalf("sent = %#v, want %#v (empty summary and absent publication time)", sent, wantSent)
	}
	wantHTML := "<b>🔔 C &lt;release&gt; &amp; update</b>\n\n<b>Source:</b> Example &amp; News\n\n" +
		"<a href=\"" + server.URL + "/releases/c?edition=1&amp;lang=en\">View original</a>"
	if !reflect.DeepEqual(rendered, []string{wantHTML}) {
		t.Fatalf("rendered = %q, want [%q]", rendered, wantHTML)
	}
	assertSeen(2, true)

	if err := pipeline.RunOnce(ctx, sourceConfig); err != nil {
		t.Fatalf("unchanged RunOnce() error = %v", err)
	}
	if len(sent) != 1 || len(rendered) != 1 {
		t.Fatalf("unchanged collection added notifications: sent=%d rendered=%d, want 1 each", len(sent), len(rendered))
	}
	for i := range items {
		assertSeen(i, true)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("HTML requests = %d, want 3 repeated collections in one process", got)
	}
}
