package html_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/source"
	htmlsource "github.com/suwa68/signal-watch/internal/source/html"
)

func TestSourcePipelineFromFileToMonitorItems(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/news" {
			http.NotFound(writer, request)
			return
		}
		_, _ = io.WriteString(writer, `
<main>
  <article class="news-item">
    <a class="title" href="/releases/one">First release</a>
    <p class="summary">Available today.</p>
  </article>
</main>`)
	}))
	t.Cleanup(server.Close)

	directory := t.TempDir()
	definition := fmt.Sprintf(`version: 1
id: example_news
name: Example News
type: html
interval_seconds: 300
config:
  url: %q
  selectors:
    item: article.news-item
    title: .title
    url: .title
    content: .summary
`, server.URL+"/news")
	if err := os.WriteFile(filepath.Join(directory, "example.yaml"), []byte(definition), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	documents, err := config.NewFileSourceDefinitionRepository(directory).List(context.Background())
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
	items, err := source.NewSourceRunner(registry).Run(context.Background(), sourceConfig)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []source.MonitorItem{{
		SourceID: "example_news",
		Title:    "First release",
		URL:      server.URL + "/releases/one",
		Content:  "Available today.",
	}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("Run() items = %#v, want %#v", items, want)
	}
}
