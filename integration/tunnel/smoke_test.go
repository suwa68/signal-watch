package tunnel_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/suwa68/signal-watch/internal/query"
)

// This test runs in the smoke image with networking disabled. The real tunnel
// client supplies a loopback control plane and launches the real stdio server.
func TestTunnelToSignalWatch(t *testing.T) {
	if os.Getenv("SIGNALWATCH_TUNNEL_INTEGRATION") != "1" {
		t.Skip("run via docker compose -f compose.tunnel.yaml run --build --rm tunnel-smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	website := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<article><a href="/update">Local tunnel update</a><p>Fixture content</p></article>`)
	}))
	defer website.Close()

	dir := t.TempDir()
	sources := filepath.Join(dir, "sources")
	if err := os.Mkdir(sources, 0700); err != nil {
		t.Fatal(err)
	}
	definition := fmt.Sprintf("version: 1\nid: local_updates\nname: Local Updates\ntype: html\ninterval_seconds: 300\nconfig:\n  url: %q\n  selectors:\n    item: article\n    title: a\n    url: a\n    content: p\n", website.URL)
	if err := os.WriteFile(filepath.Join(sources, "updates.yaml"), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	urlFile := filepath.Join(dir, "connection.json")
	logs, err := os.Create(filepath.Join(dir, "tunnel.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logs.Close()
	command := exec.CommandContext(ctx, "/usr/bin/tunnel-client", "dev", "proxy",
		"--url-file", urlFile,
		"--mcp-command", "/usr/local/bin/signalwatch-mcp --sources-dir "+sources)
	// Do not inherit runtime tunnel credentials or the image's /sources command.
	command.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/tmp"}
	command.Stdout, command.Stderr = logs, logs
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer func() {
		cancel()
		<-done
		if t.Failed() {
			data, _ := os.ReadFile(logs.Name())
			t.Logf("tunnel logs:\n%s", data)
		}
	}()

	var connection struct {
		MCPURL string `json:"mcp_url"`
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for connection.MCPURL == "" {
		data, err := os.ReadFile(urlFile)
		if err == nil {
			_ = json.Unmarshal(data, &connection)
		}
		if connection.MCPURL != "" {
			break
		}
		select {
		case err := <-done:
			done <- err // leave the result for cleanup
			t.Fatalf("local tunnel exited before readiness: %v", err)
		case <-ctx.Done():
			t.Fatal("local tunnel did not become ready:", ctx.Err())
		case <-ticker.C:
		}
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "signalwatch-tunnel-smoke", Version: "v1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: connection.MCPURL}, nil)
	if err != nil {
		t.Fatal("initialize MCP through tunnel:", err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tool := range tools.Tools {
		found[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("tool %s lost its read-only annotation", tool.Name)
		}
	}
	if len(tools.Tools) != 2 || !found["list_sources"] || !found["collect_source"] {
		t.Fatalf("unexpected tools: %v", found)
	}
	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_sources", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var catalog query.CatalogResponse
	decodeResult(t, listed, &catalog)
	if catalog.Version != 1 || len(catalog.Sources) != 1 || catalog.Sources[0].ID != "local_updates" {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	collected, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "collect_source", Arguments: map[string]any{"source_id": catalog.Sources[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot query.Snapshot
	decodeResult(t, collected, &snapshot)
	if snapshot.ReturnedItems != 1 || len(snapshot.Items) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	item := snapshot.Items[0]
	if item.Title != "Local tunnel update" || item.Content != "Fixture content" || item.URL != website.URL+"/update" {
		t.Fatalf("unexpected item: %+v", item)
	}
	failed, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "collect_source", Arguments: map[string]any{"source_id": "missing"},
	})
	if err != nil || failed == nil || !failed.IsError {
		t.Fatalf("expected a tool error for missing source: %+v, %v", failed, err)
	}
	t.Log("MCP initialize, tools/list, list_sources, collect_source, and tool errors passed through the local tunnel")
}

func decodeResult(t *testing.T, result *mcp.CallToolResult, target any) {
	t.Helper()
	if result == nil || result.IsError {
		t.Fatalf("tool failed: %+v", result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}
