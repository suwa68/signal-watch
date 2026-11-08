package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Rejecting malformed SDK requests prevents false acceptance when rendering,
// credentials, destination, parse mode, or preview options regress.
func TestTelegramContractRejectsInvalidAndRecordsFailure(t *testing.T) {
	f := newFixture()
	srv := httptest.NewServer(http.HandlerFunc(f.telegram))
	defer srv.Close()
	send := func(text, mode string) int {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		for k, v := range map[string]string{"chat_id": "-100123456", "text": text, "parse_mode": mode, "link_preview_options": `{"is_disabled":true}`} {
			_ = w.WriteField(k, v)
		}
		_ = w.Close()
		r, err := http.Post(srv.URL+"/bot123456:acceptance-token/sendMessage", w.FormDataContentType(), &body)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		return r.StatusCode
	}
	literal := "<b>🔔 C &amp; alpha</b>\n\n<b>Source:</b> alpha &amp; Co\n\n<a href=\"http://fixture:8080/items/alpha/C\">View original</a>"
	if got := send(literal, "Markdown"); got != 400 {
		t.Fatalf("invalid parse mode accepted: %d", got)
	}
	f.setMode("alpha/C", "fail")
	if got := send(literal, "HTML"); got != 400 {
		t.Fatalf("planned Telegram failure lost: %d", got)
	}
	f.setMode("alpha/C", "success")
	if got := send(literal, "HTML"); got != 200 {
		t.Fatalf("valid send rejected: %d", got)
	}
	if got := f.count("send", "alpha/C", 0); got != 2 {
		t.Fatalf("validated attempts=%d want 2", got)
	}
	if got := f.count("replied", "alpha/C", 0); got != 1 {
		t.Fatalf("successful replies=%d want 1", got)
	}
	if got := f.count("unexpected", "", 0); got != 1 {
		t.Fatalf("unexpected requests=%d want 1", got)
	}
}

// One released page must not leak into later polls, and cancellation must
// unblock the held request (the independence/shutdown barrier depends on this).
func TestOneShotPageAndCancellation(t *testing.T) {
	f := newFixture()
	srv := httptest.NewServer(http.HandlerFunc(f.html))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan string, 1)
	go func() {
		req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/alpha", nil)
		r, e := http.DefaultClient.Do(req)
		if e != nil {
			done <- e.Error()
			return
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		done <- string(b)
	}()
	if err := f.await(ctx, "held", "alpha", 0); err != nil {
		t.Fatal(err)
	}
	if err := f.page("alpha", []string{"A"}); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-done:
		if !strings.Contains(body, `href="http://fixture:8080/items/alpha/A"`) {
			t.Fatalf("page=%s", body)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cursor := f.cursor()
	reqctx, stop := context.WithCancel(ctx)
	req, _ := http.NewRequestWithContext(reqctx, "GET", srv.URL+"/alpha", nil)
	go func() {
		r, e := http.DefaultClient.Do(req)
		if e == nil {
			r.Body.Close()
		}
		done <- "done"
	}()
	if err := f.await(ctx, "held", "alpha", cursor); err != nil {
		t.Fatal(err)
	}
	stop()
	<-done
	if err := f.await(ctx, "canceled", "alpha", cursor); err != nil {
		t.Fatal(err)
	}
}

// An old success, another source's success, or a failed completion cannot
// masquerade as a post-release committed run.
func TestCompletedRunCursor(t *testing.T) {
	logs := `{"event":"startup","source_id":"alpha","status":"success"}
{"event":"source_run_completed","source_id":"alpha","status":"success"}
{"event":"source_run_completed","source_id":"beta","status":"success"}
{"event":"source_run_completed","source_id":"alpha","status":"error"}
`
	count, status, err := completedRuns(strings.NewReader(logs), "alpha")
	if err != nil || count != 2 || status != "error" {
		t.Fatalf("got %d %q %v", count, status, err)
	}
	if _, _, err := completedRuns(strings.NewReader("not-json\n"), "alpha"); err == nil {
		t.Fatal("malformed log accepted")
	}
}

func TestControlRejectsUnknownOperations(t *testing.T) {
	f := newFixture()
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"op":"bogus"}`))
	w := httptest.NewRecorder()
	f.control(w, r)
	if w.Code != 400 {
		t.Fatalf("status=%d", w.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
}

// A replacement held request must not hide an intervening timeout/cancellation
// when claiming beta progressed throughout one blocked alpha collection.
func TestHeldAssertionRejectsReplacementRequest(t *testing.T) {
	f := newFixture()
	handlerDone := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { f.html(w, r); handlerDone <- struct{}{} }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	launch := func() (context.CancelFunc, chan struct{}) {
		requestCtx, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			req, _ := http.NewRequestWithContext(requestCtx, "GET", srv.URL+"/alpha", nil)
			r, e := http.DefaultClient.Do(req)
			if e == nil {
				r.Body.Close()
			}
		}()
		return stop, done
	}
	stop, done := launch()
	if err := f.await(ctx, "held", "alpha", 0); err != nil {
		t.Fatal(err)
	}
	cursor := f.cursor()
	stop()
	<-done
	<-handlerDone
	stop, done = launch()
	defer func() { stop(); <-done }()
	if err := f.await(ctx, "held", "alpha", cursor); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"op":"assert-held","key":"alpha","after":%d}`, cursor)
	w := httptest.NewRecorder()
	f.control(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
	if w.Code != 400 {
		t.Fatal("replaced alpha request incorrectly proves continuous hold")
	}
}
