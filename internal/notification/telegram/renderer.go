package telegram

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/suwa68/signal-watch/internal/notification"
)

// MaxMessageLength is a conservative budget for the text after HTML entity
// parsing. Counting UTF-16 code units also reserves space for non-BMP emoji.
const MaxMessageLength = 3800

// Renderer formats notifications as Telegram HTML. Its zero value is ready to
// use and has no mutable state.
type Renderer struct{}

// Render preserves title and metadata and truncates only the summary, before
// escaping it. Metadata too large to leave room for a summary is rejected.
// Timestamps retain the supplied time's location; no host timezone is consulted.
func (Renderer) Render(message notification.Notification) (string, error) {
	for _, field := range []struct {
		name  string
		value string
	}{
		{"title", message.Title},
		{"summary", message.Summary},
		{"source name", message.SourceName},
	} {
		if strings.TrimSpace(field.value) == "" {
			return "", fmt.Errorf("%s is required", field.name)
		}
		if !utf8.ValidString(field.value) {
			return "", fmt.Errorf("%s must be valid UTF-8", field.name)
		}
	}
	if message.URL != "" {
		parsed, err := url.Parse(message.URL)
		if err != nil || !utf8.ValidString(message.URL) || parsed.Host == "" ||
			(parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
			return "", fmt.Errorf("URL must be an absolute HTTP/HTTPS URL without credentials")
		}
	}

	title := "🔔 " + message.Title
	footerText := "\n\nSource: " + message.SourceName
	footerHTML := "\n\n<b>Source:</b> " + html.EscapeString(message.SourceName)
	if message.PublishedAt != nil {
		published := message.PublishedAt.Format("2006-01-02 15:04 MST")
		footerText += "\nPublished: " + published
		footerHTML += "\n<b>Published:</b> " + html.EscapeString(published)
	}
	if message.URL != "" {
		footerText += "\n\nView original"
		footerHTML += "\n\n<a href=\"" + html.EscapeString(message.URL) + "\">View original</a>"
	}

	summaryBudget := MaxMessageLength - textLength(title+"\n\n"+footerText)
	if summaryBudget < 1 {
		return "", fmt.Errorf("title/source metadata leaves no summary space within the %d-unit Telegram limit", MaxMessageLength)
	}
	summary := truncateSummary(message.Summary, summaryBudget)
	return "<b>" + html.EscapeString(title) + "</b>\n\n" + html.EscapeString(summary) + footerHTML, nil
}

func textLength(text string) int {
	length := 0
	for _, r := range text {
		length += utf16.RuneLen(r)
	}
	return length
}

func truncateSummary(summary string, budget int) string {
	if textLength(summary) <= budget {
		return summary
	}
	remaining := budget - 1 // Reserve one unit for the ellipsis.
	for index, r := range summary {
		remaining -= utf16.RuneLen(r)
		if remaining < 0 {
			return summary[:index] + "…"
		}
	}
	return summary
}
