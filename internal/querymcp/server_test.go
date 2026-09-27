package querymcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/suwa68/signal-watch/internal/query"
)

func TestToolsAreDiscoverableAndCallableWithStructuredResults(t *testing.T) {
	t.Parallel()
	website := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`<article><a href="/current">Current update</a><p>Selected text</p></article>`))
	}))
	defer website.Close()
	catalog, service := loadTestCatalog(t, website.URL)
	server, err := NewServer(catalog, service, DefaultTimeout)
	if err != nil {
		t.Fatal(err)
	}
	session := connectTestClient(t, server)

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("tools = %#v", tools.Tools)
	}
	found := map[string]bool{}
	for _, tool := range tools.Tools {
		found[tool.Name] = true
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("tool %q is missing an explicit schema", tool.Name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q annotations = %#v", tool.Name, tool.Annotations)
		}
	}
	if !found["list_sources"] || !found["collect_source"] {
		t.Fatalf("tool names = %#v", found)
	}

	listed, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_sources", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("list_sources = %#v, %v", listed, err)
	}
	listedMap, ok := listed.StructuredContent.(map[string]any)
	if !ok || listedMap["version"] != float64(1) {
		t.Fatalf("list_sources structured content = %#v", listed.StructuredContent)
	}

	collected, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "collect_source", Arguments: map[string]any{"source_id": "updates", "max_items": 1, "max_content_chars": 100},
	})
	if err != nil || collected.IsError {
		t.Fatalf("collect_source = %#v, %v", collected, err)
	}
	collectedMap, ok := collected.StructuredContent.(map[string]any)
	if !ok || collectedMap["returned_items"] != float64(1) {
		t.Fatalf("collect_source structured content = %#v", collected.StructuredContent)
	}

	failed, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "collect_source", Arguments: map[string]any{"source_id": "missing"}})
	if callErr == nil && (failed == nil || !failed.IsError) {
		t.Fatalf("missing source was not a tool failure: %#v, %v", failed, callErr)
	}
	failed, callErr = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_sources", Arguments: map[string]any{"unexpected": true}})
	if callErr == nil && (failed == nil || !failed.IsError) {
		t.Fatalf("unknown argument was not rejected: %#v, %v", failed, callErr)
	}
}

func TestCollectSourceAppliesServerTimeout(t *testing.T) {
	t.Parallel()
	website := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer website.Close()
	catalog, service := loadTestCatalog(t, website.URL)
	server, err := NewServer(catalog, service, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	session := connectTestClient(t, server)
	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "collect_source", Arguments: map[string]any{"source_id": "updates"}})
	if callErr == nil && (result == nil || !result.IsError) {
		t.Fatalf("timed-out call did not fail: %#v, %v", result, callErr)
	}
}

func loadTestCatalog(t *testing.T, websiteURL string) (*query.Catalog, *query.Service) {
	t.Helper()
	directory := t.TempDir()
	definition := fmt.Sprintf("version: 1\nid: updates\nname: Updates\ntype: html\ninterval_seconds: 300\nconfig:\n  url: %q\n  selectors:\n    item: article\n    title: a\n    url: a\n    content: p\n", websiteURL)
	if err := os.WriteFile(filepath.Join(directory, "updates.yaml"), []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, service, err := query.LoadHTMLCatalog(context.Background(), directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	return catalog, service
}

func connectTestClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "signalwatch-query-test", Version: "v1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}
