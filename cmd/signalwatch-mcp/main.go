// Command signalwatch-mcp serves Query v1 over MCP stdio.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/suwa68/signal-watch/internal/query"
	"github.com/suwa68/signal-watch/internal/querymcp"
)

func main() {
	flags := flag.NewFlagSet("signalwatch-mcp", flag.ContinueOnError)
	sourcesDir := flags.String("sources-dir", "", "directory containing source YAML definitions")
	timeout := flags.Duration("timeout", querymcp.DefaultTimeout, "startup and per-collection timeout")
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if strings.TrimSpace(*sourcesDir) == "" || *timeout <= 0 || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "--sources-dir is required, --timeout must be positive, and positional arguments are not accepted")
		os.Exit(2)
	}

	caller, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	loadCtx, cancelLoad := context.WithTimeout(caller, *timeout)
	catalog, service, err := query.LoadHTMLCatalog(loadCtx, *sourcesDir, nil)
	cancelLoad()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load source catalog: %v\n", err)
		os.Exit(2)
	}
	server, err := querymcp.NewServer(catalog, service, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := server.Run(caller, &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "serve MCP: %v\n", err)
		os.Exit(1)
	}
}
