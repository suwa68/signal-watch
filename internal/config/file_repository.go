package config

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var ErrSourceDefinitionDirectoryRequired = errors.New("source definition directory is required")

// FileSourceDefinitionRepository reads source definitions from one directory.
// Version 1 intentionally does not recurse into subdirectories or follow
// symbolic links.
type FileSourceDefinitionRepository struct {
	directory string
}

func NewFileSourceDefinitionRepository(directory string) *FileSourceDefinitionRepository {
	return &FileSourceDefinitionRepository{directory: directory}
}

func (r *FileSourceDefinitionRepository) List(ctx context.Context) ([]SourceDefinitionDocument, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil || strings.TrimSpace(r.directory) == "" {
		return nil, ErrSourceDefinitionDirectoryRequired
	}

	directory, err := filepath.Abs(r.directory)
	if err != nil {
		return nil, fmt.Errorf("resolve source definition directory %q: %w", r.directory, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("list source definitions in %q: %w", directory, err)
	}

	slices.SortFunc(entries, func(left, right os.DirEntry) int {
		return strings.Compare(left.Name(), right.Name())
	})

	documents := make([]SourceDefinitionDocument, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.Type().IsRegular() || !isYAMLFilename(entry.Name()) {
			continue
		}

		origin := filepath.Join(directory, entry.Name())
		content, err := os.ReadFile(origin)
		if err != nil {
			return nil, fmt.Errorf("read source definition %q: %w", origin, err)
		}
		revision := sha256.Sum256(content)
		documents = append(documents, SourceDefinitionDocument{
			Key:      entry.Name(),
			Content:  content,
			Revision: fmt.Sprintf("%x", revision),
			Origin:   origin,
		})
	}

	return documents, nil
}

func isYAMLFilename(name string) bool {
	extension := filepath.Ext(name)
	return extension == ".yaml" || extension == ".yml"
}

var _ SourceDefinitionRepository = (*FileSourceDefinitionRepository)(nil)
