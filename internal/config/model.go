// Package config defines source-definition loading and configuration contracts.
package config

// SourceDefinitionDocument is the storage-neutral representation returned by a
// source-definition repository.
type SourceDefinitionDocument struct {
	Key      string
	Content  []byte
	Revision string
	Origin   string
}

// DecodedConfig is a serialization-neutral source definition.
type DecodedConfig map[string]any

// SourceConfig is the validated, source-type-neutral runtime configuration.
// Adapter-specific values stay inside Config.
type SourceConfig struct {
	ID              string
	Name            string
	Type            string
	IntervalSeconds int
	Config          map[string]any
}
