package config

// ConfigDecoder answers how a source definition is serialized.
type ConfigDecoder interface {
	Decode(document SourceDefinitionDocument) (DecodedConfig, error)
}
