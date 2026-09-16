package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/suwa68/signal-watch/internal/notification"
)

type senderFunc func(context.Context, *bot.SendMessageParams) (*models.Message, error)

func (f senderFunc) SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	return f(ctx, params)
}

func TestNotifierPassesMessageOptionsAndContext(t *testing.T) {
	t.Parallel()
	for _, chatID := range []string{"-1001234567890", "@test_channel"} {
		t.Run(chatID, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), struct{}{}, "request"), time.Second)
			defer cancel()
			message := exampleNotification()
			wantText, err := (Renderer{}).Render(message)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			n := &Notifier{client: senderFunc(func(gotCtx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
				calls++
				if gotCtx != ctx {
					t.Error("caller context was not passed through")
				}
				if params.ChatID != chatID || params.Text != wantText || params.ParseMode != models.ParseModeHTML {
					t.Errorf("unexpected sendMessage params: %+v", params)
				}
				if params.LinkPreviewOptions == nil || params.LinkPreviewOptions.IsDisabled == nil || !*params.LinkPreviewOptions.IsDisabled {
					t.Error("link previews must be disabled")
				}
				return &models.Message{}, nil
			})}
			if err := n.Send(ctx, Destination{ID: "news", ChatID: chatID}, message); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("SendMessage calls = %d, want 1", calls)
			}
		})
	}
}

func TestNotifierPreservesSDKErrorWithoutRetry(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("SDK failure")
	calls := 0
	n := &Notifier{client: senderFunc(func(context.Context, *bot.SendMessageParams) (*models.Message, error) {
		calls++
		return nil, wantErr
	})}
	err := n.Send(context.Background(), Destination{ID: "news", ChatID: "@test_channel"}, exampleNotification())
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), `destination "news"`) {
		t.Fatalf("Send() error = %v, want wrapped SDK failure and destination", err)
	}
	if calls != 1 {
		t.Fatalf("SendMessage calls = %d, want 1", calls)
	}
}

func TestNotifierValidationPreventsSDKCall(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name        string
		destination Destination
		message     notification.Notification
	}{
		{"invalid notification", Destination{ID: "news", ChatID: "@test_channel"}, notification.Notification{}},
		{"missing destination ID", Destination{ChatID: "@test_channel"}, exampleNotification()},
		{"missing chat ID", Destination{ID: "news", ChatID: " \t"}, exampleNotification()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			n := &Notifier{client: senderFunc(func(context.Context, *bot.SendMessageParams) (*models.Message, error) {
				t.Error("SDK called for invalid input")
				return nil, nil
			})}
			if err := n.Send(context.Background(), tt.destination, tt.message); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNotifierCanceledContextPreventsSDKCall(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n := &Notifier{client: senderFunc(func(context.Context, *bot.SendMessageParams) (*models.Message, error) {
		t.Error("SDK called with canceled context")
		return nil, nil
	})}
	err := n.Send(ctx, Destination{ID: "news", ChatID: "@test_channel"}, exampleNotification())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send() error = %v, want context.Canceled", err)
	}
}

func TestNotifierRequiresInitialization(t *testing.T) {
	t.Parallel()
	for _, n := range []*Notifier{nil, {}} {
		if err := n.Send(context.Background(), Destination{}, exampleNotification()); err == nil {
			t.Fatal("expected initialization error")
		}
	}
	if _, err := New(" \t"); err == nil {
		t.Fatal("expected missing token error without network access")
	}
}

func TestSDKInitializationAndOutboundRequest(t *testing.T) {
	t.Parallel()
	var getMeCalls, sendCalls atomic.Int32
	message := exampleNotification()
	message.Summary = strings.Repeat("外部 <text> & 🚀 ", 1000)
	wantText, err := (Renderer{}).Render(message)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		switch r.URL.Path {
		case "/bottest-token/getMe":
			getMeCalls.Add(1)
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test"}}`)
		case "/bottest-token/sendMessage":
			sendCalls.Add(1)
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			defer r.MultipartForm.RemoveAll()
			if r.FormValue("chat_id") != "@test_channel" || r.FormValue("text") != wantText || r.FormValue("parse_mode") != "HTML" {
				t.Error("SDK request does not contain expected destination, rendered text, and HTML mode")
			}
			var preview struct {
				IsDisabled bool `json:"is_disabled"`
			}
			if err := json.Unmarshal([]byte(r.FormValue("link_preview_options")), &preview); err != nil || !preview.IsDisabled {
				t.Errorf("link_preview_options = %q, error = %v", r.FormValue("link_preview_options"), err)
			}
			_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
		default:
			t.Errorf("unexpected API method: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	n, err := newNotifier("test-token", bot.WithServerURL(server.URL), bot.WithHTTPClient(time.Second, server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Send(context.Background(), Destination{ID: "news", ChatID: "@test_channel"}, message); err != nil {
		t.Fatal(err)
	}
	if getMeCalls.Load() != 1 || sendCalls.Load() != 1 {
		t.Fatalf("getMe calls = %d, sendMessage calls = %d, want one each", getMeCalls.Load(), sendCalls.Load())
	}
}

func TestSDKAPIErrorsArePreserved(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		status  int
		wantErr error
	}{
		{400, bot.ErrorBadRequest},
		{401, bot.ErrorUnauthorized},
		{403, bot.ErrorForbidden},
		{429, nil},
	} {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = fmt.Fprintf(w, `{"ok":false,"error_code":%d,"description":"test failure","parameters":{"retry_after":7}}`, tt.status)
			}))
			t.Cleanup(server.Close)
			n, err := newNotifier("test-token", bot.WithServerURL(server.URL), bot.WithSkipGetMe(), bot.WithHTTPClient(time.Second, server.Client()))
			if err != nil {
				t.Fatal(err)
			}
			err = n.Send(context.Background(), Destination{ID: "news", ChatID: "@test_channel"}, exampleNotification())
			if err == nil || !strings.Contains(err.Error(), `destination "news"`) {
				t.Fatalf("Send() error = %v, want destination context", err)
			}
			if tt.status == 429 {
				var rateLimit *bot.TooManyRequestsError
				if !errors.As(err, &rateLimit) || rateLimit.RetryAfter != 7 {
					t.Fatalf("lost SDK rate limit details: %v", err)
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Send() error = %v, want %v", err, tt.wantErr)
			}
			if calls.Load() != 1 {
				t.Fatalf("requests = %d, want 1 (no retry)", calls.Load())
			}
		})
	}
}

func TestSDKInitializationFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/getMe" {
			t.Errorf("unexpected API method: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":401,"description":"Unauthorized"}`)
	}))
	t.Cleanup(server.Close)
	n, err := newNotifier("test-token", bot.WithServerURL(server.URL), bot.WithHTTPClient(time.Second, server.Client()))
	if n != nil || !errors.Is(err, bot.ErrorUnauthorized) {
		t.Fatalf("newNotifier() = %v, %v, want wrapped startup failure", n, err)
	}
}

func TestSDKInFlightCancellation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Consume the body so net/http can observe the client disconnect.
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
			t.Error("request context was not canceled")
		}
	}))
	t.Cleanup(server.Close)
	n, err := newNotifier("test-token", bot.WithServerURL(server.URL), bot.WithSkipGetMe(), bot.WithHTTPClient(time.Second, server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- n.Send(ctx, Destination{ID: "news", ChatID: "@test_channel"}, exampleNotification())
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("request did not reach test server")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Send() error = %v, want context.Canceled", err)
		}
		if strings.Contains(err.Error(), "test-token") {
			t.Fatal("SDK transport error exposed the bot token")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send did not return after cancellation")
	}
}
