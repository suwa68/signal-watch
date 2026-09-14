package config

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFileSourceDefinitionRepositoryListsYAMLFiles(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	files := map[string]string{
		"b.yml":       "not: [valid YAML",
		"a.yaml":      "version: 1\n",
		"ignored.txt": "version: 1\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(directory, "nested.yaml"), 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "nested.yaml", "source.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(nested) error = %v", err)
	}

	documents, err := NewFileSourceDefinitionRepository(directory).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(documents), 2; got != want {
		t.Fatalf("List() returned %d documents, want %d", got, want)
	}
	if got, want := []string{documents[0].Key, documents[1].Key}, []string{"a.yaml", "b.yml"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("document keys = %v, want %v", got, want)
	}

	for _, document := range documents {
		content := files[document.Key]
		if got := string(document.Content); got != content {
			t.Errorf("document %q content = %q, want %q", document.Key, got, content)
		}
		if got, want := document.Origin, filepath.Join(directory, document.Key); got != want {
			t.Errorf("document %q origin = %q, want %q", document.Key, got, want)
		}
		wantRevision := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
		if document.Revision != wantRevision {
			t.Errorf("document %q revision = %q, want %q", document.Key, document.Revision, wantRevision)
		}
	}
}

func TestFileSourceDefinitionRepositoryReportsDirectoryErrors(t *testing.T) {
	t.Parallel()

	_, err := NewFileSourceDefinitionRepository(filepath.Join(t.TempDir(), "missing")).List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "list source definitions") {
		t.Fatalf("List() error = %v, want directory context", err)
	}
}

func TestFileSourceDefinitionRepositoryRequiresDirectory(t *testing.T) {
	t.Parallel()

	_, err := NewFileSourceDefinitionRepository("  ").List(context.Background())
	if !errors.Is(err, ErrSourceDefinitionDirectoryRequired) {
		t.Fatalf("List() error = %v, want ErrSourceDefinitionDirectoryRequired", err)
	}
}

func TestFileSourceDefinitionRepositoryPropagatesCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewFileSourceDefinitionRepository(t.TempDir()).List(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
}
