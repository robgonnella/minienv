package loader

import (
	"context"

	"github.com/robgonnella/minienv/internal/config"
)

// LoadComposeProject exposes the compose load on its own. LoadCore returns a
// *core.Core whose fields are private, so nothing else can assert on what
// compose resolved — notably ${VAR} interpolation reaching extension values.
func (l *Loader) LoadComposeProject(
	ctx context.Context,
) (*config.ComposeProject, error) {
	return l.loadComposeProject(ctx)
}
