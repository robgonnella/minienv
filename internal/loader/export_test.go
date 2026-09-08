package loader

import (
	"context"

	"github.com/robgonnella/minienv/internal/config"
)

// LoadComposeProject exposes the compose load on its own. LoadCore returns a
// *core.Core whose fields are private, so this is the only way to assert on
// what compose resolved — notably that ${VAR} interpolation reaches extension
// values, which per-environment ngrok URLs depend on.
func (l *Loader) LoadComposeProject(
	ctx context.Context,
) (*config.ComposeProject, error) {
	return l.loadComposeProject(ctx)
}
