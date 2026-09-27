package query

import (
	"context"
	"errors"
	"testing"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/source"
)

type repositoryFunc func(context.Context) ([]config.SourceDefinitionDocument, error)

func (f repositoryFunc) List(ctx context.Context) ([]config.SourceDefinitionDocument, error) {
	return f(ctx)
}

func TestLoadCatalogValidatesAllDefinitionsAndSorts(t *testing.T) {
	t.Parallel()
	documents := []config.SourceDefinitionDocument{
		{Key: "z.yaml", Revision: "rz", Content: []byte("version: 1\nid: z\nname: Z\ntype: fake\ninterval_seconds: 1\nconfig: {}\n")},
		{Key: "a.yaml", Revision: "ra", Content: []byte("version: 1\nid: a\nname: A\ntype: fake\ninterval_seconds: 1\nconfig: {}\n")},
	}
	registry := source.NewSourceAdapterRegistry()
	if err := registry.Register("fake", sourceAdapterFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) { return nil, nil })); err != nil {
		t.Fatal(err)
	}
	validated := 0
	catalog, err := LoadCatalog(context.Background(), repositoryFunc(func(context.Context) ([]config.SourceDefinitionDocument, error) { return documents, nil }), config.NewYAMLConfigDecoder(), registry, map[string]DefinitionValidator{
		"fake": DefinitionValidatorFunc(func(config.SourceConfig) (DefinitionMetadata, error) { validated++; return DefinitionMetadata{}, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := CatalogResult(catalog)
	if validated != 2 || len(got.Sources) != 2 || got.Sources[0].ID != "a" || got.Sources[0].DefinitionRevision != "ra" || got.Sources[1].ID != "z" {
		t.Fatalf("catalog = %#v, validated = %d", got, validated)
	}
}

type sourceAdapterFunc func(context.Context, config.SourceConfig) ([]source.MonitorItem, error)

func (f sourceAdapterFunc) Collect(ctx context.Context, cfg config.SourceConfig) ([]source.MonitorItem, error) {
	return f(ctx, cfg)
}

func TestLoadCatalogRejectsDuplicateAndInvalidUnselectedDefinition(t *testing.T) {
	t.Parallel()
	registry := source.NewSourceAdapterRegistry()
	_ = registry.Register("fake", sourceAdapterFunc(func(context.Context, config.SourceConfig) ([]source.MonitorItem, error) { return nil, nil }))
	valid := []byte("version: 1\nid: same\nname: Same\ntype: fake\ninterval_seconds: 1\nconfig: {}\n")
	for _, test := range []struct {
		name      string
		documents []config.SourceDefinitionDocument
		want      error
	}{
		{"duplicate", []config.SourceDefinitionDocument{{Key: "a", Content: valid}, {Key: "b", Content: valid}}, ErrDuplicateID},
		{"invalid later definition", []config.SourceDefinitionDocument{{Key: "a", Content: valid}, {Key: "b", Content: []byte("version: 1\nid: broken")}}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog, err := LoadCatalog(context.Background(), repositoryFunc(func(context.Context) ([]config.SourceDefinitionDocument, error) { return test.documents, nil }), config.NewYAMLConfigDecoder(), registry, map[string]DefinitionValidator{"fake": DefinitionValidatorFunc(func(config.SourceConfig) (DefinitionMetadata, error) { return DefinitionMetadata{}, nil })})
			if err == nil || catalog != nil || (test.want != nil && !errors.Is(err, test.want)) {
				t.Fatalf("LoadCatalog() = %#v, %v", catalog, err)
			}
		})
	}
}
