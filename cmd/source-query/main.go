// Command source-query collects one configured source and writes Query JSON v1.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/suwa68/signal-watch/internal/query"
)

const (
	exitOK      = 0
	exitRuntime = 1
	exitConfig  = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("source-query", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sourcesDir := flags.String("sources-dir", "", "directory containing source YAML definitions")
	sourceID := flags.String("source-id", "", "exact configured source ID")
	timeout := flags.Duration("timeout", 15*time.Second, "loading and collection timeout")
	maxItems := flags.Int("max-items", query.DefaultMaxItems, "maximum returned items (1-100)")
	maxContentChars := flags.Int("max-content-chars", query.DefaultMaxContentChars, "maximum Unicode characters per item content (100-5000)")
	if err := flags.Parse(args); err != nil {
		return exitConfig
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return exitConfig
	}
	if strings.TrimSpace(*sourcesDir) == "" {
		fmt.Fprintln(stderr, "--sources-dir is required")
		return exitConfig
	}
	if strings.TrimSpace(*sourceID) == "" {
		fmt.Fprintln(stderr, "--source-id is required")
		return exitConfig
	}
	if *timeout <= 0 {
		fmt.Fprintln(stderr, "--timeout must be greater than zero")
		return exitConfig
	}
	limits := query.Limits{MaxItems: *maxItems, MaxContentChars: *maxContentChars}
	if err := limits.Validate(); err != nil {
		fmt.Fprintln(stderr, err)
		return exitConfig
	}

	caller, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(caller, *timeout)
	defer cancel()
	catalog, service, err := query.LoadHTMLCatalog(ctx, *sourcesDir, nil)
	if err != nil {
		fmt.Fprintf(stderr, "load source catalog: %v\n", err)
		return exitConfig
	}
	definition, err := catalog.Lookup(*sourceID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitConfig
	}
	snapshot, err := service.Collect(ctx, definition, limits)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(stderr, "query canceled: %v\n", err)
		} else {
			fmt.Fprintln(stderr, err)
		}
		return exitRuntime
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(snapshot); err != nil {
		fmt.Fprintf(stderr, "encode query result: %v\n", err)
		return exitRuntime
	}
	return exitOK
}
