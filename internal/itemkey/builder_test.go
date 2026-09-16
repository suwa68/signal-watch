package itemkey

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/suwa68/signal-watch/internal/source"
)

func TestBuildUsesFallbackPriority(t *testing.T) {
	t.Parallel()

	published := time.Date(2026, time.September, 15, 18, 30, 0, 123, time.FixedZone("UTC+8", 8*60*60))
	tests := []struct {
		name string
		item source.MonitorItem
		want string
	}{
		{
			name: "external ID wins",
			item: source.MonitorItem{
				ExternalID:  "123",
				URL:         "https://example.com/news/123",
				PublishedAt: &published,
				Title:       "MLCC Update",
				Content:     "Details",
			},
			want: "external:123",
		},
		{
			name: "URL wins without external ID",
			item: source.MonitorItem{
				URL:         "https://example.com/news/123",
				PublishedAt: &published,
				Title:       "MLCC Update",
				Content:     "Details",
			},
			want: "url:https://example.com/news/123",
		},
		{
			name: "published time and title win before content",
			item: source.MonitorItem{
				PublishedAt: &published,
				Title:       "MLCC Update",
				Content:     "Details",
			},
			want: "published:2026-09-15T10:30:00.000000123Z:title:MLCC Update",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := Build(test.item)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("Build() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuildUsesDeterministicContentFallback(t *testing.T) {
	t.Parallel()

	item := source.MonitorItem{Title: "Status", Content: "Available"}
	first, err := Build(item)
	if err != nil {
		t.Fatalf("Build() first error = %v", err)
	}
	second, err := Build(item)
	if err != nil {
		t.Fatalf("Build() second error = %v", err)
	}
	if first != second {
		t.Fatalf("Build() keys differ: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "content:") || len(first) != len("content:")+64 {
		t.Fatalf("Build() = %q, want content: followed by SHA-256", first)
	}

	different, err := Build(source.MonitorItem{Title: "Status", Content: "Unavailable"})
	if err != nil {
		t.Fatalf("Build() different error = %v", err)
	}
	if first == different {
		t.Fatal("Build() returned the same key for different content")
	}
}

func TestBuildContentFallbackKeepsFieldBoundary(t *testing.T) {
	t.Parallel()

	left, err := Build(source.MonitorItem{Title: "ab", Content: "c"})
	if err != nil {
		t.Fatalf("Build() left error = %v", err)
	}
	right, err := Build(source.MonitorItem{Title: "a", Content: "bc"})
	if err != nil {
		t.Fatalf("Build() right error = %v", err)
	}
	if left == right {
		t.Fatal("Build() did not preserve the Title/Content boundary")
	}
}

func TestBuildFallsBackFromUnusablePublishedTime(t *testing.T) {
	t.Parallel()

	zero := time.Time{}
	key, err := Build(source.MonitorItem{PublishedAt: &zero, Title: "Title"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.HasPrefix(key, "content:") {
		t.Fatalf("Build() = %q, want content fallback", key)
	}
}

func TestBuildRejectsMissingDeterministicIdentity(t *testing.T) {
	t.Parallel()

	_, err := Build(source.MonitorItem{Title: "  ", Content: "\n\t"})
	if !errors.Is(err, ErrNoDeterministicIdentity) {
		t.Fatalf("Build() error = %v, want ErrNoDeterministicIdentity", err)
	}
}
