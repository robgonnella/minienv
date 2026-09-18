package loader

import (
	"context"

	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
)

// LoadComposeProject exposes the compose load on its own. LoadCore returns a
// *core.Core whose fields are private, so nothing else can assert on what
// compose resolved — notably ${VAR} interpolation reaching extension values.
func (l *Loader) LoadComposeProject(
	ctx context.Context,
) (*config.ComposeProject, error) {
	return l.loadComposeProject(ctx)
}

// LoadServiceDirs exposes the include walk on its own. The map it builds only
// reaches the deployers through their options, so nothing else can assert
// which directory a service was attributed to.
func (l *Loader) LoadServiceDirs(
	ctx context.Context,
	project *config.ComposeProject,
) (deployer.ServiceDirs, error) {
	return l.loadServiceDirs(ctx, project)
}
