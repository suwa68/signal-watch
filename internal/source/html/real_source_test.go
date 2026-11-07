package html_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/suwa68/signal-watch/internal/query"
	"github.com/suwa68/signal-watch/internal/source"
	htmlsource "github.com/suwa68/signal-watch/internal/source/html"
)

// Changing the committed source selectors to a broad anchor selector must fail:
// the fixture includes author, navigation, and abbreviated-SHA link decoys.
func TestRealSourceCommittedYAMLThroughRunner(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/real_source_commits.html")
	if err != nil {
		t.Fatal(err)
	}
	want := []source.MonitorItem{
		{
			SourceID: "signalwatch_commits",
			Title:    "Run SignalWatch MCP tunnel entirely in Docker (#14)",
			URL:      "https://github.com/suwa68/signal-watch/commit/6950ade914918082c67aa6f08fda3e7a2aab862a",
		},
		{
			SourceID: "signalwatch_commits",
			Title:    "feat: add AI-callable source Query v1 (#12)",
			URL:      "https://github.com/suwa68/signal-watch/commit/f60075c75992a2ba499a3ad3ce352bfade870fd2",
		},
	}
	for _, test := range []struct {
		name string
		html string
		want []source.MonitorItem
		err  error
	}{
		{name: "ordered commit titles and resolved URLs", html: string(fixture), want: want},
		{name: "no matching rows", html: `<main><a href="/explore">Explore</a></main>`, want: []source.MonitorItem{}},
		{
			name: "missing required title rejects partial collection",
			html: string(fixture) + `<li data-testid="commit-row-item"><a href="/suwa68">Author only</a></li>`,
			err:  htmlsource.ErrMissingTitle,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			definition, _, client := realSourceFixture(t, test.html)
			registry := source.NewSourceAdapterRegistry()
			if err := registry.Register("html", htmlsource.New(client)); err != nil {
				t.Fatal(err)
			}
			items, err := source.NewSourceRunner(registry).Run(context.Background(), definition.Config)
			if !errors.Is(err, test.err) {
				t.Fatalf("Run() error = %v, want %v", err, test.err)
			}
			if !reflect.DeepEqual(items, test.want) {
				t.Fatalf("Run() items = %#v, want %#v", items, test.want)
			}
		})
	}
}

// The public query must retain the configured URL and report unavailable content
// honestly even when the page has unrelated text and publication time markup.
func TestRealSourceCommittedYAMLQuery(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/real_source_commits.html")
	if err != nil {
		t.Fatal(err)
	}
	definition, service, _ := realSourceFixture(t, string(fixture))
	snapshot, err := service.Collect(context.Background(), definition, query.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TotalItems != 2 || snapshot.ReturnedItems != 2 || snapshot.Truncated {
		t.Fatalf("unexpected counts: %#v", snapshot)
	}
	if snapshot.Source.ConfiguredSourceURL == nil || *snapshot.Source.ConfiguredSourceURL != "https://github.com/suwa68/signal-watch/commits/main" {
		t.Fatalf("unexpected configured URL: %#v", snapshot.Source)
	}
	for _, item := range snapshot.Items {
		if item.Content != "" || item.ContentStatus != "empty" || item.ContentCompleteness != "unknown" || item.ContentTruncated || item.PublishedAt != nil || item.ExternalID != "" {
			t.Fatalf("unexpected optional metadata: %#v", item)
		}
	}
}

func realSourceFixture(t *testing.T, markup string) (query.Definition, *query.Service, *http.Client) {
	t.Helper()
	client := &http.Client{Transport: realSourceTransport{markup: markup}}
	catalog, service, err := query.LoadHTMLCatalog(context.Background(), "../../../examples/sources", client)
	if err != nil {
		t.Fatalf("LoadHTMLCatalog() error = %v", err)
	}
	definition, err := catalog.Lookup("signalwatch_commits")
	if err != nil {
		t.Fatalf("committed real source is unavailable: %v", err)
	}
	return definition, service, client
}

// No fallback transport exists: these tests cannot make a network request.
// Keeping the configured URL intact exercises the public URL and relative hrefs.
type realSourceTransport struct{ markup string }

func (f realSourceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method != http.MethodGet || request.URL.String() != "https://github.com/suwa68/signal-watch/commits/main" {
		return nil, fmt.Errorf("unexpected fixture request: %s %s", request.Method, request.URL)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(f.markup)),
		Request:    request,
	}, nil
}
