package config

import "context"

// SourceDefinitionRepository answers where source definitions come from.
type SourceDefinitionRepository interface {
	List(ctx context.Context) ([]SourceDefinitionDocument, error)
}
