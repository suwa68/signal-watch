package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The acceptance assertion must distinguish one canceled, unreleased request
// from a still-pending request, a released success, or multiple attempts.
func TestUncertainSendRequiresOneCanceledUnreleasedAttempt(t *testing.T) {
	for _, phase := range []string{"held", "canceled", "released", "repeated", "stale cursor"} {
		t.Run(phase, func(t *testing.T) {
			f := newFixture()
			f.setMode("alpha/U", "hold")
			server := httptest.NewServer(http.HandlerFunc(f.telegram))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			control := func(op string, after int) int {
				t.Helper()
				w := httptest.NewRecorder()
				body := fmt.Sprintf(`{"op":%q,"key":"alpha/U","after":%d}`, op, after)
				f.control(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
				return w.Code
			}
			send := func(after int) (context.CancelFunc, chan struct{}) {
				t.Helper()
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				for k, v := range map[string]string{"chat_id": "-100123456", "parse_mode": "HTML", "link_preview_options": `{"is_disabled":true}`, "text": "<b>🔔 U &amp; alpha</b>\n\n<b>Source:</b> alpha &amp; Co\n\n<a href=\"http://fixture:8080/items/alpha/U\">View original</a>"} {
					if err := writer.WriteField(k, v); err != nil {
						t.Fatal(err)
					}
				}
				writer.Close()
				requestCtx, stop := context.WithCancel(ctx)
				done := make(chan struct{})
				req, err := http.NewRequestWithContext(requestCtx, "POST", server.URL+"/bot123456:acceptance-token/sendMessage", &body)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", writer.FormDataContentType())
				go func() {
					defer close(done)
					r, err := http.DefaultClient.Do(req)
					if err == nil {
						io.Copy(io.Discard, r.Body)
						r.Body.Close()
					}
				}()
				if err := f.await(ctx, "send", "alpha/U", after); err != nil {
					stop()
					t.Fatal(err)
				}
				return stop, done
			}
			before := f.cursor()
			stop, done := send(before)
			if code := control("assert-send-held", before); code != 200 {
				t.Errorf("held assertion=%d want200", code)
			}
			switch phase {
			case "released":
				if code := control("release", before); code != 200 {
					t.Fatal("release failed")
				}
				<-done
				if f.count("released", "alpha/U", before) != 1 {
					t.Error("response release missing from evidence ledger")
				}
			case "canceled", "repeated", "stale cursor":
				stop()
				<-done
				if err := f.await(ctx, "send-canceled", "alpha/U", before); err != nil {
					t.Fatal(err)
				}
				if phase == "repeated" {
					next := f.cursor()
					stop, done = send(next)
					stop()
					<-done
					if err := f.await(ctx, "send-canceled", "alpha/U", next); err != nil {
						t.Fatal(err)
					}
				}
				if phase == "stale cursor" {
					before = f.cursor()
				}
			}
			want := 400
			if phase == "canceled" {
				want = 200
			}
			if code := control("assert-uncertain-send", before); code != want {
				t.Errorf("uncertain assertion=%d want%d", code, want)
			}
			stop()
			<-done
		})
	}
}
