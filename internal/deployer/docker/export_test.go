package docker

import (
	"context"

	"github.com/robgonnella/minienv/internal/compose"
)

// InitProject exposes the half of Init that follows the compose load. Init
// itself loads the project from a Source on disk, so this is the only way to
// hand a spec-built project to the deployer.
func (d *Docker) InitProject(
	ctx context.Context,
	project compose.Project,
	dirs compose.ServiceDirs,
) error {
	if err := d.validateExtension(); err != nil {
		return err
	}

	return d.initProject(ctx, project, dirs)
}
