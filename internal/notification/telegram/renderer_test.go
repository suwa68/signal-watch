package telegram

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/suwa68/signal-watch/internal/notification"
)

func exampleNotification() notification.Notification {
	published := time.Date(2026, 9, 15, 10, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	return notification.Notification{
		Title:       "Murata Announces MLCC Product Changes",
		Summary:     "Murata has announced adjustments to selected MLCC product lines.",
		SourceName:  "Murata News",
		URL:         "https://example.com/news/123",
		PublishedAt: &published,
	}
}

func TestRendererFormatAndOptionalFields(t *testing.T) {
	t.Parallel()
	const head = "<b>🔔 Murata Announces MLCC Product Changes</b>\n\n" +
		"Murata has announced adjustments to selected MLCC product lines.\n\n" +
		"<b>Source:</b> Murata News"
	const published = "\n<b>Published:</b> 2026-09-15 10:30 CST"
	const link = "\n\n<a href=\"https://example.com/news/123\">View original</a>"
	for _, tt := range []struct {
		name     string
		withURL  bool
		withTime bool
		want     string
	}{
		{"all fields", true, true, head + published + link},
		{"without URL", false, true, head + published},
		{"without timestamp", true, false, head + link},
		{"required only", false, false, head},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			message := exampleNotification()
			if !tt.withURL {
				message.URL = ""
			}
			if !tt.withTime {
				message.PublishedAt = nil
			}
			for range 2 {
				got, err := (Renderer{}).Render(message)
				if err != nil {
					t.Fatal(err)
				}
				if got != tt.want {
					t.Fatalf("Render() = %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestRendererEscapesAllExternalText(t *testing.T) {
	t.Parallel()
	const hostile = `Murata <MLCC> & "EOL" 'news'`
	const escaped = `Murata &lt;MLCC&gt; &amp; &#34;EOL&#34; &#39;news&#39;`
	message := notification.Notification{
		Title:      hostile,
		Summary:    hostile,
		SourceName: hostile,
		URL:        `https://example.com/news?q="MLCC"&tag=<EOL>&name='news'`,
	}
	got, err := (Renderer{}).Render(message)
	if err != nil {
		t.Fatal(err)
	}
	want := "<b>🔔 " + escaped + "</b>\n\n" + escaped + "\n\n<b>Source:</b> " + escaped +
		"\n\n<a href=\"https://example.com/news?q=&#34;MLCC&#34;&amp;tag=&lt;EOL&gt;&amp;name=&#39;news&#39;\">View original</a>"
	if got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
	visible, links := parseRendered(t, got)
	if want := "🔔 " + hostile + "\n\n" + hostile + "\n\nSource: " + hostile + "\n\nView original"; visible != want {
		t.Fatalf("visible text = %q, want %q", visible, want)
	}
	if len(links) != 1 || links[0] != message.URL {
		t.Fatalf("links = %q, want original URL", links)
	}
}

func TestRendererUsesSuppliedTimezone(t *testing.T) {
	t.Parallel()
	message := exampleNotification()
	utc := message.PublishedAt.UTC()
	message.PublishedAt = &utc
	got, err := (Renderer{}).Render(message)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<b>Published:</b> 2026-09-15 02:30 UTC") {
		t.Fatalf("unexpected timestamp: %s", got)
	}
}

func TestRendererRejectsInvalidFields(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		change func(*notification.Notification)
		field  string
	}{
		{"empty title", func(n *notification.Notification) { n.Title = "" }, "title"},
		{"blank summary", func(n *notification.Notification) { n.Summary = " \n\t" }, "summary"},
		{"empty source", func(n *notification.Notification) { n.SourceName = "" }, "source name"},
		{"invalid title UTF-8", func(n *notification.Notification) { n.Title = "\xff" }, "UTF-8"},
		{"invalid summary UTF-8", func(n *notification.Notification) { n.Summary = "\xff" }, "UTF-8"},
		{"invalid source UTF-8", func(n *notification.Notification) { n.SourceName = "\xff" }, "UTF-8"},
		{"oversized title", func(n *notification.Notification) { n.Title = strings.Repeat("T", MaxMessageLength) }, "metadata"},
		{"oversized source", func(n *notification.Notification) { n.SourceName = strings.Repeat("S", MaxMessageLength) }, "metadata"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			message := exampleNotification()
			tt.change(&message)
			got, err := (Renderer{}).Render(message)
			if err == nil || !strings.Contains(err.Error(), tt.field) || got != "" {
				t.Fatalf("Render() = %q, %v, want %s validation error", got, err, tt.field)
			}
		})
	}
	for _, invalidURL := range []string{"/relative", "javascript:alert(1)", "https:///missing-host", "https://user:password@example.com", "https://example.com/\n", "://", "https://example.com/\xff"} {
		t.Run(invalidURL, func(t *testing.T) {
			t.Parallel()
			message := exampleNotification()
			message.URL = invalidURL
			if _, err := (Renderer{}).Render(message); err == nil || !strings.Contains(err.Error(), "URL") {
				t.Fatalf("Render() error = %v, want URL validation error", err)
			}
		})
	}
}

func TestRendererTruncatesSummaryWithoutBreakingHTMLOrUnicode(t *testing.T) {
	t.Parallel()
	for _, summary := range []string{
		strings.Repeat("x", 5000),
		strings.Repeat("中文摘要", 2000),
		strings.Repeat("🔔🚀🌏", 2000),
		strings.Repeat("<&>\"'🚀", 2000),
	} {
		t.Run(summary[:1], func(t *testing.T) {
			t.Parallel()
			message := exampleNotification()
			message.Summary = summary
			got, err := (Renderer{}).Render(message)
			if err != nil {
				t.Fatal(err)
			}
			visible, links := parseRendered(t, got)
			if !utf8.ValidString(got) {
				t.Fatal("rendered HTML is not valid UTF-8")
			}
			if length := len(utf16.Encode([]rune(visible))); length > MaxMessageLength || length < MaxMessageLength-1 {
				t.Fatalf("visible length = %d, want %d or %d", length, MaxMessageLength-1, MaxMessageLength)
			}
			prefix := "🔔 " + message.Title + "\n\n"
			suffix := "\n\nSource: Murata News\nPublished: 2026-09-15 10:30 CST\n\nView original"
			if !strings.HasPrefix(visible, prefix) || !strings.HasSuffix(visible, suffix) {
				t.Fatal("title or metadata changed during truncation")
			}
			shortened := strings.TrimSuffix(strings.TrimPrefix(visible, prefix), suffix)
			if !strings.HasSuffix(shortened, "…") || !strings.HasPrefix(summary, strings.TrimSuffix(shortened, "…")) {
				t.Fatal("summary must be an original prefix followed by an ellipsis")
			}
			if len(links) != 1 || links[0] != message.URL {
				t.Fatalf("link was damaged: %v", links)
			}
		})
	}
}

func TestRendererSummaryBudgetBoundaries(t *testing.T) {
	t.Parallel()
	message := notification.Notification{Title: "T", Summary: "x", SourceName: "S"}
	rendered, err := (Renderer{}).Render(message)
	if err != nil {
		t.Fatal(err)
	}
	visible, _ := parseRendered(t, rendered)
	budget := MaxMessageLength - (len(utf16.Encode([]rune(visible))) - 1)
	for _, size := range []int{budget - 1, budget, budget + 1} {
		message.Summary = strings.Repeat("&", size)
		got, err := (Renderer{}).Render(message)
		if err != nil {
			t.Fatal(err)
		}
		visible, _ := parseRendered(t, got)
		wantSummary := message.Summary
		if size > budget {
			wantSummary = strings.Repeat("&", budget-1) + "…"
		}
		if want := "🔔 T\n\n" + wantSummary + "\n\nSource: S"; visible != want {
			t.Fatalf("wrong boundary output for summary length %d", size)
		}
	}
	// Leave only one or two units for a supplementary Unicode character.
	for _, remaining := range []int{0, 1, 2} {
		message.Title = "T" + strings.Repeat("t", budget-remaining)
		message.Summary = "🚀🚀"
		got, err := (Renderer{}).Render(message)
		if remaining == 0 {
			if err == nil {
				t.Fatal("expected error when metadata consumes the entire budget")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		visible, _ := parseRendered(t, got)
		if !strings.HasSuffix(visible, "\n\n…\n\nSource: S") || len(utf16.Encode([]rune(visible))) > MaxMessageLength {
			t.Fatal("small budget must produce only a complete ellipsis")
		}
	}
}

// Parse independently of the renderer to check balanced markup, decoded text,
// and attribute escaping, including around truncation boundaries.
func parseRendered(t *testing.T, rendered string) (string, []string) {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader("<root>" + rendered + "</root>"))
	var visible strings.Builder
	var links []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid rendered markup: %v", err)
		}
		switch token := token.(type) {
		case xml.CharData:
			visible.Write(token)
		case xml.StartElement:
			switch token.Name.Local {
			case "root", "b":
				if len(token.Attr) != 0 {
					t.Fatalf("unexpected attributes on %s", token.Name.Local)
				}
			case "a":
				if len(token.Attr) != 1 || token.Attr[0].Name.Local != "href" {
					t.Fatal("unexpected anchor attributes")
				}
				links = append(links, token.Attr[0].Value)
			default:
				t.Fatalf("unexpected tag: %s", token.Name.Local)
			}
		}
	}
	return visible.String(), links
}
