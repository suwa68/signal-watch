package source

// MonitorItem is the provisional normalized output of a source adapter.
//
// Identity, hashing, and change semantics are intentionally outside this v1
// model while those domain contracts remain unresolved.
type MonitorItem struct {
	SourceID string
	Title    string
	URL      string
	Content  string
}
