package source

import (
	"context"

	"github.com/suwa68/signal-watch/internal/config"
)

// SourceAdapter collects normalized monitoring items for one source.
type SourceAdapter interface {
	Collect(ctx context.Context, source config.SourceConfig) ([]MonitorItem, error)
}
