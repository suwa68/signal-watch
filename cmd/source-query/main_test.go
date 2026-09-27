package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/suwa68/signal-watch/internal/query"
)

func TestRunFromYAMLThroughHTMLToJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`<article><a href="/one">One</a><p>Current text</p></article>`))
	}))
	defer server.Close()
	directory := t.TempDir()
	definition := fmt.Sprintf("version: 1\nid: updates\nname: Updates\ntype: html\ninterval_seconds: 300\nconfig:\n  url: %q\n  selectors:\n    item: article\n    title: a\n    url: a\n    content: p\n", server.URL)
	if err := os.WriteFile(filepath.Join(directory, "updates.yaml"), []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--sources-dir", directory, "--source-id", "updates"}, &stdout, &stderr)
	if code != exitOK || stderr.Len() != 0 {
		t.Fatalf("run() code=%d stderr=%q", code, stderr.String())
	}
	var snapshot query.Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snapshot); err != nil {
		t.Fatalf("stdout is not one JSON document: %v; %q", err, stdout.String())
	}
	if snapshot.Version != 1 || snapshot.Source.ID != "updates" || snapshot.Source.DefinitionRevision == "" || snapshot.ReturnedItems != 1 || snapshot.Items[0].URL != server.URL+"/one" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestRunClassifiesConfigurationBeforeCollection(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	valid := "version: 1\nid: selected\nname: Selected\ntype: html\ninterval_seconds: 1\nconfig:\n  url: https://example.com\n  selectors:\n    item: article\n    title: a\n"
	invalid := "version: 1\nid: unselected\nname: Invalid\ntype: html\ninterval_seconds: 1\nconfig:\n  url: javascript:bad\n  selectors:\n    item: article\n    title: a\n"
	for name, content := range map[string]string{"valid.yaml": valid, "invalid.yaml": invalid} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--sources-dir", directory, "--source-id", "selected"}, &stdout, &stderr); code != exitConfig || stdout.Len() != 0 {
		t.Fatalf("run() code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
