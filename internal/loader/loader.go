package loader

import (
	"context"
	"slices"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/go-viper/mapstructure/v2"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/core"
	"github.com/robgonnella/minienv/internal/deployer"
	"github.com/robgonnella/minienv/internal/os"
	"github.com/rs/zerolog/log"
)

type LoaderOpts struct {
	Files            []string
	ProjectDirectory string
	ProjectName      string
	DryRun           bool
	Commander        os.Commander
}

type Loader struct {
	opts *LoaderOpts
}

func New(opts *LoaderOpts) *Loader {
	return &Loader{opts}
}

func (l *Loader) LoadCore() (*core.Core, error) {
	project, err := l.loadComposeProject()
	if err != nil {
		return nil, err
	}

	ext, err := l.loadMainExtensionConfig(project)
	if err != nil {
		return nil, err
	}

	deployer, err := l.loadActiveDeployer(ext)
	if err != nil {
		return nil, err
	}

	return core.New(ext, project, deployer, l.opts.DryRun), nil
}

func (l *Loader) loadComposeProject() (*config.ComposeProject, error) {
	// Same option set the compose CLI builds in its own toProjectOptions, so
	// project name, .env loading, COMPOSE_FILE and default config-file discovery
	// all resolve exactly the way `docker compose` resolves them.
	projectOpts, err := composecli.NewProjectOptions(
		l.opts.Files,
		composecli.WithWorkingDirectory(l.opts.ProjectDirectory),
		composecli.WithOsEnv,
		composecli.WithDotEnv,
		composecli.WithConfigFileEnv,
		composecli.WithDefaultConfigPath,
		composecli.WithName(l.opts.ProjectName),
	)
	if err != nil {
		return nil, Errorf("failed to create compose project options: %s", err)
	}

	project, err := projectOpts.LoadProject(context.Background())
	if err != nil {
		return nil, Errorf("failed to load compose project: %s", err)
	}

	return project, nil
}

func (l *Loader) loadMainExtensionConfig(
	project *config.ComposeProject,
) (*config.XMiniEnv, error) {
	ex, ok := project.Extensions[config.TOP_LEVEL_EXTENSION]
	if !ok {
		return nil, Errorf(
			"no minienv extension config found in docker compose configs",
		)
	}

	var extConfig config.XMiniEnv
	if err := mapstructure.Decode(ex, &extConfig); err != nil {
		return nil, Errorf(
			"failed to parse x-minienv top-level extension: %s",
			err,
		)
	}

	log.
		Info().
		Interface(config.TOP_LEVEL_EXTENSION, ex).
		Msg("loaded top-level extension")

	return &extConfig, nil
}

func (l *Loader) loadActiveDeployer(
	ext *config.XMiniEnv,
) (deployer.Deployer, error) {
	deployers := []deployer.Deployer{
		deployer.NewHelm(
			ext,
			l.opts.Commander,
			config.NGROK_AUTHTOKEN,
			l.opts.DryRun,
		),
	}

	var targetDeployer deployer.Deployer

	activeDeployers := []string{}
	for _, d := range deployers {
		if d.Active() {
			activeDeployers = append(activeDeployers, d.ConfigField())
			targetDeployer = d
		}
	}

	if len(activeDeployers) > 1 {
		return nil, Errorf(
			"detected multiple active configurations for deployment. "+
				"only one of [%s] can be configured",
			strings.Join(activeDeployers, ", "),
		)
	}

	if targetDeployer == nil {
		return nil, Errorf(
			"failed to find an active configuration for deployment. "+
				"configure one of [%s] in x-minienv extension field",
			slices.Collect(func(yield func(s string) bool) {
				for _, d := range deployers {
					if !yield(d.ConfigField()) {
						return
					}
				}
			}),
		)
	}

	return targetDeployer, nil
}
