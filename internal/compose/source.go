package compose

import (
	"context"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/robgonnella/minienv/internal/errs"
)

type Source struct {
	Files            []string
	ProjectDirectory string
	ProjectName      string
}

func Load(ctx context.Context, src Source) (*Project, error) {
	// Same option set the compose CLI builds in its own toProjectOptions, so
	// project name, .env loading, COMPOSE_FILE and default config-file discovery
	// all resolve exactly the way `docker compose` resolves them.
	projectOpts, err := composecli.NewProjectOptions(
		src.Files,
		composecli.WithWorkingDirectory(src.ProjectDirectory),
		composecli.WithOsEnv,
		composecli.WithDotEnv,
		composecli.WithConfigFileEnv,
		composecli.WithDefaultConfigPath,
		composecli.WithName(src.ProjectName),
	)
	if err != nil {
		return nil, errs.Errorf(
			ErrProjectOptions,
			"failed to create compose project options: %w",
			err,
		)
	}

	project, err := projectOpts.LoadProject(ctx)
	if err != nil {
		return nil, errs.Errorf(
			ErrProjectLoad,
			"failed to load compose project: %w",
			err,
		)
	}

	return project, nil
}
