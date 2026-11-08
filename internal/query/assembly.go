package query

import (
	"context"
	"net/http"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/source"
	htmlsource "github.com/suwa68/signal-watch/internal/source/html"
)

// LoadHTMLCatalog assembles the current native adapters and validates a file catalog.
func LoadHTMLCatalog(ctx context.Context, directory string, client *http.Client) (*Catalog, *Service, error) {
	catalog, runner, err := LoadHTMLCatalogAndRunner(ctx, directory, client)
	if err != nil {
		return nil, nil, err
	}
	service, err := NewService(runner)
	if err != nil {
		return nil, nil, err
	}
	return catalog, service, nil
}

// LoadHTMLCatalogAndRunner shares validated native HTML composition with monitoring.
// The runner collects normalized items; it never accesses monitoring state.
func LoadHTMLCatalogAndRunner(ctx context.Context, directory string, client *http.Client) (*Catalog, *source.SourceRunner, error) {
	registry := source.NewSourceAdapterRegistry()
	if err := registry.Register("html", htmlsource.New(client)); err != nil {
		return nil, nil, err
	}
	validators := map[string]DefinitionValidator{
		"html": DefinitionValidatorFunc(func(cfg config.SourceConfig) (DefinitionMetadata, error) {
			settings, err := htmlsource.ParseSettings(cfg.Config)
			if err != nil {
				return DefinitionMetadata{}, err
			}
			configuredURL := settings.URL
			return DefinitionMetadata{ConfiguredSourceURL: &configuredURL}, nil
		}),
	}
	catalog, err := LoadCatalog(ctx, config.NewFileSourceDefinitionRepository(directory), config.NewYAMLConfigDecoder(), registry, validators)
	if err != nil {
		return nil, nil, err
	}
	return catalog, source.NewSourceRunner(registry), nil
}
