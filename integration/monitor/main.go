// Command monitor-fixture is offline test infrastructure. It is never copied
// into the production monitor image and exposes no host ports.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const token = "123456:acceptance-token"
const chat = "-100123456"

type event struct {
	Seq  int    `json:"seq"`
	Kind string `json:"kind"`
	Key  string `json:"key"`
}
type fixture struct {
	mu       sync.Mutex
	events   []event
	changed  chan struct{}
	pages    map[string]chan []string
	modes    map[string]string
	releases map[string]chan struct{}
	held     map[string]int
}

func newFixture() *fixture {
	f := &fixture{changed: make(chan struct{}), pages: map[string]chan []string{}, modes: map[string]string{}, releases: map[string]chan struct{}{}, held: map[string]int{}}
	for _, s := range []string{"alpha", "beta", "empty"} {
		f.pages[s] = make(chan []string, 1)
	}
	return f
}
func (f *fixture) record(kind, key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recordLocked(kind, key)
}
func (f *fixture) recordLocked(kind, key string) {
	f.events = append(f.events, event{len(f.events) + 1, kind, key})
	close(f.changed)
	f.changed = make(chan struct{})
}
func (f *fixture) cursor() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.events) }
func (f *fixture) count(kind, key string, after int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.countLocked(kind, key, after)
}
func (f *fixture) countLocked(kind, key string, after int) int {
	n := 0
	for _, e := range f.events {
		if e.Seq > after && e.Kind == kind && (key == "" || e.Key == key) {
			n++
		}
	}
	return n
}
func (f *fixture) await(ctx context.Context, kind, key string, after int) error {
	for {
		f.mu.Lock()
		n := f.countLocked(kind, key, after)
		ch := f.changed
		f.mu.Unlock()
		if n > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s %s after %d: %w", kind, key, after, ctx.Err())
		case <-ch:
		}
	}
}
func (f *fixture) page(source string, items []string) error {
	ch, ok := f.pages[source]
	if !ok {
		return errors.New("unknown source")
	}
	for _, item := range items {
		if !regexp.MustCompile(`^[A-Z]$`).MatchString(item) {
			return errors.New("items must be single uppercase letters")
		}
	}
	select {
	case ch <- items:
		return nil
	default:
		return errors.New("page already queued")
	}
}
func (f *fixture) setMode(key, mode string) { f.mu.Lock(); defer f.mu.Unlock(); f.modes[key] = mode }
func (f *fixture) html(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")
	ch, ok := f.pages[key]
	if !ok || r.Method != "GET" {
		f.reject(w)
		return
	}
	f.mu.Lock()
	f.held[key]++
	f.recordLocked("held", key)
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.held[key]--; f.mu.Unlock() }()
	select {
	case <-r.Context().Done():
		f.record("canceled", key)
	case items := <-ch:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		for _, item := range items {
			fmt.Fprintf(w, `<article><a href="http://fixture:8080/items/%s/%s">%s</a><h2>%s</h2></article>`, key, item, item, html.EscapeString(item+" & "+key))
		}
		f.record("source-response", key)
	}
}
func (f *fixture) reject(w http.ResponseWriter) {
	f.record("unexpected", "")
	http.Error(w, `{"ok":false,"error_code":400,"description":"invalid fixture request"}`, 400)
}

var itemLink = regexp.MustCompile(`http://fixture:8080/items/(alpha|beta|empty)/([A-Z])`)

func (f *fixture) telegram(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		f.reject(w)
		return
	}
	switch r.URL.Path {
	case "/bot" + token + "/getMe":
		f.record("getme", "")
		io.WriteString(w, `{"ok":true,"result":{"id":123456,"is_bot":true,"first_name":"Acceptance"}}`)
	case "/bot" + token + "/sendMessage":
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		if err := r.ParseMultipartForm(16384); err != nil {
			f.reject(w)
			return
		}
		defer r.MultipartForm.RemoveAll()
		text := r.FormValue("text")
		m := itemLink.FindStringSubmatch(text)
		var preview struct {
			Disabled bool `json:"is_disabled"`
		}
		if len(m) != 3 || r.FormValue("chat_id") != chat || r.FormValue("parse_mode") != "HTML" || json.Unmarshal([]byte(r.FormValue("link_preview_options")), &preview) != nil || !preview.Disabled {
			f.reject(w)
			return
		}
		expected := fmt.Sprintf("<b>🔔 %s &amp; %s</b>\n\n<b>Source:</b> %s &amp; Co\n\n<a href=\"http://fixture:8080/items/%s/%s\">View original</a>", m[2], m[1], m[1], m[1], m[2])
		if text != expected {
			f.reject(w)
			return
		}
		key := m[1] + "/" + m[2]
		f.mu.Lock()
		mode := f.modes[key]
		var release chan struct{}
		if mode == "hold" {
			release = make(chan struct{})
			f.releases[key] = release
		}
		f.recordLocked("send", key)
		f.mu.Unlock()
		if release != nil {
			select {
			case <-r.Context().Done():
				f.record("send-canceled", key)
				return
			case <-release:
			}
		}
		if mode == "fail" {
			w.WriteHeader(400)
			io.WriteString(w, `{"ok":false,"error_code":400,"description":"planned fixture failure"}`)
			f.record("failed", key)
			return
		}
		if _, err := io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`); err == nil {
			f.record("replied", key)
		}
	default:
		f.reject(w)
	}
}

type controlRequest struct {
	Op       string         `json:"op"`
	Kind     string         `json:"kind"`
	Key      string         `json:"key"`
	After    int            `json:"after"`
	Items    string         `json:"items"`
	Mode     string         `json:"mode"`
	Count    int            `json:"count"`
	Attempts map[string]int `json:"attempts"`
	Replies  map[string]int `json:"replies"`
}

func (f *fixture) control(w http.ResponseWriter, r *http.Request) {
	var req controlRequest
	d := json.NewDecoder(io.LimitReader(r.Body, 16384))
	d.DisallowUnknownFields()
	if r.Method != "POST" || d.Decode(&req) != nil {
		writeResult(w, nil, errors.New("invalid control request"))
		return
	}
	var result any = true
	var err error
	switch req.Op {
	case "ready":
		err = checkReady()
	case "cursor":
		result = f.cursor()
	case "ledger":
		f.mu.Lock()
		result = append([]event{}, f.events...)
		f.mu.Unlock()
	case "await":
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		err = f.await(ctx, req.Kind, req.Key, req.After)
	case "page":
		var items []string
		if req.Items != "" {
			items = strings.Split(req.Items, ",")
		}
		err = f.page(req.Key, items)
	case "mode":
		if req.Mode != "fail" && req.Mode != "success" && req.Mode != "hold" {
			err = errors.New("invalid send mode")
		} else {
			f.setMode(req.Key, req.Mode)
		}
	case "release":
		f.mu.Lock()
		ch, ok := f.releases[req.Key]
		if ok {
			delete(f.releases, req.Key)
			f.recordLocked("released", req.Key)
			close(ch)
		} else {
			err = errors.New("no held send")
		}
		f.mu.Unlock()
	case "assert-send-held", "assert-uncertain-send":
		f.mu.Lock()
		canceled := 0
		if req.Op == "assert-uncertain-send" {
			canceled = 1
		}
		_, held := f.releases[req.Key]
		if !held || f.countLocked("send", req.Key, req.After) != 1 || f.countLocked("send-canceled", req.Key, req.After) != canceled || f.countLocked("released", req.Key, req.After) != 0 || f.countLocked("replied", req.Key, req.After) != 0 || f.countLocked("failed", req.Key, req.After) != 0 {
			err = errors.New("expected exactly one unreleased send with the requested cancellation state")
		}
		f.mu.Unlock()
	case "getme-count":
		result = f.count("getme", "", 0)
	case "assert-getme":
		if f.count("getme", "", 0) != req.Count {
			err = errors.New("second owner reached getMe")
		}
	case "assert-held":
		f.mu.Lock()
		if f.held[req.Key] != 1 || f.countLocked("held", req.Key, req.After) != 0 || f.countLocked("canceled", req.Key, req.After) != 0 {
			err = errors.New("source was not continuously held after cursor")
		}
		f.mu.Unlock()
	case "assert-canceled":
		f.mu.Lock()
		if f.countLocked("canceled", req.Key, req.After) != 1 || f.countLocked("held", req.Key, req.After) != 0 || f.countLocked("source-response", req.Key, req.After) != 0 {
			err = errors.New("expected cancellation of exactly the pre-signal held request")
		}
		f.mu.Unlock()
	case "assert":
		err = f.assertLedger(req.Attempts, req.Replies)
	default:
		err = errors.New("unknown control operation")
	}
	writeResult(w, result, err)
}
func writeResult(w http.ResponseWriter, result any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(400)
		result = map[string]string{"error": err.Error()}
	}
	_ = json.NewEncoder(w).Encode(result)
}
func (f *fixture) assertLedger(attempts, replies map[string]int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.countLocked("unexpected", "", 0) != 0 {
		return errors.New("unexpected SDK/source requests recorded")
	}
	for kind, want := range map[string]map[string]int{"send": attempts, "replied": replies} {
		got := map[string]int{}
		for _, e := range f.events {
			if e.Kind == kind {
				got[e.Key]++
			}
		}
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		if string(a) != string(b) {
			return fmt.Errorf("%s got %s want %s", kind, a, b)
		}
	}
	return nil
}
