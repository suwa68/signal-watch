// Package querymcp exposes Query v1 through MCP without owning query semantics.
package querymcp

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/suwa68/signal-watch/internal/query"
)

const DefaultTimeout = 15 * time.Second

type listSourcesInput struct{}

type collectSourceInput struct {
	SourceID        string `json:"source_id" jsonschema:"Configured source ID returned by list_sources"`
	MaxItems        *int   `json:"max_items,omitempty" jsonschema:"Maximum returned items, from 1 through 100"`
	MaxContentChars *int   `json:"max_content_chars,omitempty" jsonschema:"Maximum Unicode characters per returned item content, from 100 through 5000"`
}

// NewServer creates a read-only stdio-compatible MCP server over an immutable catalog.
func NewServer(catalog *query.Catalog, service *query.Service, timeout time.Duration) (*mcp.Server, error) {
	if catalog == nil || service == nil {
		return nil, fmt.Errorf("catalog and query service are required")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("MCP collection timeout must be greater than zero")
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "signalwatch-query", Version: "v1"}, &mcp.ServerOptions{
		Instructions: "Use list_sources to discover configured source IDs, then collect_source for current data. Content may be incomplete and publication times may be absent. These tools never send notifications.",
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_sources",
		Title:       "List configured sources",
		Description: "Discover configured SignalWatch sources without fetching website content or sending notifications.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listSourcesInput) (*mcp.CallToolResult, query.CatalogResponse, error) {
		return nil, query.CatalogResult(catalog), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "collect_source",
		Title:       "Collect a configured source",
		Description: "Collect current items from one configured SignalWatch source. Content can be incomplete, PublishedAt can be absent, and this read-only tool does not use seen state or send notifications.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input collectSourceInput) (*mcp.CallToolResult, query.Snapshot, error) {
		definition, err := catalog.Lookup(input.SourceID)
		if err != nil {
			return nil, query.Snapshot{}, err
		}
		limits := query.DefaultLimits()
		if input.MaxItems != nil {
			limits.MaxItems = *input.MaxItems
		}
		if input.MaxContentChars != nil {
			limits.MaxContentChars = *input.MaxContentChars
		}
		if err := limits.Validate(); err != nil {
			return nil, query.Snapshot{}, err
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		snapshot, err := service.Collect(callCtx, definition, limits)
		return nil, snapshot, err
	})
	return server, nil
}

func boolPointer(value bool) *bool { return &value }
