package loader

import (
	"context"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/go-viper/mapstructure/v2"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/core"
	"github.com/robgonnella/minienv/internal/deployer"
	"github.com/robgonnella/minienv/internal/deployer/docker"
	"github.com/robgonnella/minienv/internal/deployer/helm"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/robgonnella/minienv/internal/image"
	"github.com/robgonnella/minienv/internal/publishing"
	"github.com/robgonnella/minienv/internal/transport"
	"github.com/rs/zerolog/log"
)

type LoaderOpts struct {
	Files            []string
	ProjectDirectory string
	ProjectName      string
	DryRun           bool
	ImageClient      image.Client
	GitClient        git.Client
	PublishClient    publishing.Client
	NgrokAuthToken   string
	HelmDriver       string
}

type Loader struct {
	opts *LoaderOpts
}

func New(opts *LoaderOpts) *Loader {
	return &Loader{opts}
}

func (l *Loader) LoadCore(ctx context.Context) (*core.Core, error) {
	project, err := l.loadComposeProject(ctx)
	if err != nil {
		return nil, err
	}

	ext, err := l.loadMainExtensionConfig(project)
	if err != nil {
		return nil, err
	}

	walk, err := l.walkServices(ctx, project)
	if err != nil {
		return nil, err
	}

	deployer, err := l.loadActiveDeployer(
		ext,
		walk.dirs,
		walk.hostEnv(project),
	)
	if err != nil {
		return nil, err
	}

	return core.New(*project, deployer), nil
}

func (l *Loader) loadComposeProject(
	ctx context.Context,
) (*config.ComposeProject, error) {
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

// compose merges included files and drops which one declared each service.
// Extension paths resolve against that file, so the walk recovers it.
func (l *Loader) walkServices(
	ctx context.Context,
	project *config.ComposeProject,
) (*serviceDirWalk, error) {
	walk := newServiceDirWalk(project.Name)

	if err := walk.visit(
		ctx,
		project.ComposeFiles,
		project.WorkingDir,
		project.Environment,
		nil,
	); err != nil {
		return nil, err
	}

	for name := range project.Services {
		if _, ok := walk.dirs[name]; !ok {
			return nil, errs.Errorf(
				ErrServiceDirMissing,
				"failed to find the compose file declaring service %s",
				name,
			)
		}
	}

	return walk, nil
}

// only exists to facilitate testing.
func (l *Loader) loadServiceDirs(
	ctx context.Context,
	project *config.ComposeProject,
) (deployer.ServiceDirs, error) {
	walk, err := l.walkServices(ctx, project)
	if err != nil {
		return nil, err
	}

	return walk.dirs, nil
}

// only exists to facilitate testing.
func (l *Loader) loadHostEnv(
	ctx context.Context,
	project *config.ComposeProject,
) (deployer.HostEnv, error) {
	walk, err := l.walkServices(ctx, project)
	if err != nil {
		return nil, err
	}

	return walk.hostEnv(project), nil
}

func (l *Loader) loadMainExtensionConfig(
	project *config.ComposeProject,
) (*config.XMiniEnv, error) {
	ex, ok := project.Extensions[config.TopLevelExtension]
	if !ok {
		return nil, errs.Errorf(
			ErrNoExtension,
			"no minienv extension config found in docker compose configs",
		)
	}

	var extConfig config.XMiniEnv
	if err := mapstructure.Decode(ex, &extConfig); err != nil {
		return nil, errs.Errorf(
			ErrExtensionDecode,
			"failed to parse x-minienv top-level extension: %w",
			err,
		)
	}

	log.
		Info().
		Interface(config.TopLevelExtension, ex).
		Msg("loaded top-level extension")

	return &extConfig, nil
}

// Counting the configured targets before building any of them keeps
// "configure only one deployer" ahead of whatever a half-configured deployer
// would complain about first.
func (l *Loader) loadActiveDeployer(
	ext *config.XMiniEnv,
	serviceDirs deployer.ServiceDirs,
	hostEnv deployer.HostEnv,
) (deployer.Deployer, error) {
	configured := 0

	if ext.K8s != nil {
		configured++
	}

	if ext.Docker != nil {
		configured++
	}

	if configured > 1 {
		return nil, errs.Errorf(
			ErrMultipleDeployers,
			"detected multiple active configurations for deployment. "+
				"only one of [%s] can be configured",
			strings.Join(ext.ConfigFields(), ", "),
		)
	}

	switch {
	case ext.K8s != nil:
		return helm.New(helm.Options{
			K8sExt:         *ext.K8s,
			ServiceDirs:    serviceDirs,
			HostEnv:        hostEnv,
			ImageClient:    l.opts.ImageClient,
			GitClient:      l.opts.GitClient,
			PublishClient:  l.opts.PublishClient,
			NgrokAuthToken: l.opts.NgrokAuthToken,
			HelmDriver:     l.opts.HelmDriver,
			DryRun:         l.opts.DryRun,
		}), nil
	case ext.Docker != nil:
		transport, err := l.loadActiveDockerTransport(ext.Docker)
		if err != nil {
			return nil, err
		}

		return docker.New(docker.Options{
			DockerExt:      *ext.Docker,
			ServiceDirs:    serviceDirs,
			Transport:      transport,
			ImageClient:    l.opts.ImageClient,
			GitClient:      l.opts.GitClient,
			PublishClient:  l.opts.PublishClient,
			NgrokAuthToken: l.opts.NgrokAuthToken,
			DryRun:         l.opts.DryRun,
		}), nil
	default:
		return nil, errs.Errorf(
			ErrNoActiveDeployer,
			"failed to find an active configuration for deployment. "+
				"configure one of [%s] in x-minienv extension field",
			strings.Join(ext.ConfigFields(), ", "),
		)
	}
}

func (l *Loader) loadActiveDockerTransport(
	ext *config.XMiniEnvDocker,
) (transport.Client, error) {
	activeTransports := []transport.Client{}

	if ext.Transport.SSH != nil {
		sshTransport, err := transport.NewSSHTransport(
			transport.SSHTransportOptions{Config: *ext.Transport.SSH},
		)
		if err != nil {
			return nil, err
		}

		activeTransports = append(activeTransports, sshTransport)
	}

	if len(activeTransports) > 1 {
		return nil, errs.Errorf(
			ErrMultipleDockerTransports,
			"detected multiple active configurations for docker transport. "+
				"only one of [%s] can be configured",
			strings.Join(ext.Transport.ConfigFields(), ", "),
		)
	}

	if len(activeTransports) == 0 {
		return nil, errs.Errorf(
			ErrNoActiveDockerTransport,
			"failed to find an active configuration for docker transport. "+
				"configure one of [%s] in the transport field of the docker "+
				"x-minienv extension",
			strings.Join(ext.Transport.ConfigFields(), ", "),
		)
	}

	return activeTransports[0], nil
}
