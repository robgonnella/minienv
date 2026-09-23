package resolver

import (
	"context"
	"math"
	"path"
	"path/filepath"

	"github.com/robgonnella/minienv/internal/compose"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/rs/zerolog/log"
)

type DockerServiceOptions struct {
	DockerExt    config.XMiniEnvDocker
	Service      compose.Service
	Dir          string
	GitClient    git.Client
	NgrokEnabled bool
}

type DockerService struct {
	config.XMiniEnvDockerService

	Compose compose.Service
	Dir     string
}

func NewDockerService(
	ctx context.Context,
	opts DockerServiceOptions,
) (*DockerService, error) {
	svc := opts.Service

	log.Info().Str("service", svc.Name).Msg("loading service extension")

	rawSvcExt, ok := svc.Extensions[config.DockerServiceExtension]
	if !ok {
		rawSvcExt = map[string]any{}
	}

	svcExt := &DockerService{Compose: svc, Dir: opts.Dir}

	if err := decodeServiceExtension(
		rawSvcExt,
		"x-minienv-docker-service",
		&svcExt.XMiniEnvDockerService,
	); err != nil {
		return nil, err
	}

	if err := svcExt.resolve(
		ctx,
		opts.DockerExt,
		svc,
		opts.GitClient,
		opts.NgrokEnabled,
	); err != nil {
		return nil, err
	}

	log.
		Info().
		Msgf(
			"resolved %s extension: %+v",
			config.DockerServiceExtension,
			svcExt.XMiniEnvDockerService,
		)

	return svcExt, nil
}

func (s *DockerService) CopyLocalPath(c config.XMiniEnvDockerCopy) string {
	return filepath.Join(s.Dir, c.HostPath)
}

func (s *DockerService) resolve(
	ctx context.Context,
	dockerExt config.XMiniEnvDocker,
	svc compose.Service,
	gitClient git.Client,
	ngrokEnabled bool,
) error {
	if err := s.resolveServiceImage(ctx, svc, gitClient); err != nil {
		return err
	}

	if err := s.resolveCopy(); err != nil {
		return err
	}

	return s.resolveNgrok(svc, dockerExt, ngrokEnabled)
}

func (s *DockerService) resolveCopy() error {
	targets := map[string]bool{}

	for i, declared := range s.Copy {
		cleaned := filepath.Clean(declared.HostPath)

		// Clean("") is ".", which IsLocal accepts.
		if cleaned == "." || !filepath.IsLocal(cleaned) {
			return errs.Errorf(
				ErrCopyHostPath,
				"copy hostPath must name a path inside the project: %s",
				declared.HostPath,
			)
		}

		if !path.IsAbs(declared.ContainerPath) {
			return errs.Errorf(
				ErrCopyContainerPath,
				"copy containerPath must be an absolute path: %s",
				declared.ContainerPath,
			)
		}

		if targets[declared.ContainerPath] {
			return errs.Errorf(
				ErrCopyContainerPath,
				"copy containerPath is already mounted by this service: %s",
				declared.ContainerPath,
			)
		}

		targets[declared.ContainerPath] = true
		s.Copy[i].HostPath = cleaned
	}

	return nil
}

func (s *DockerService) resolveNgrok(
	svc compose.Service,
	dockerExt config.XMiniEnvDocker,
	ngrokEnabled bool,
) error {
	targets := []uint16{}

	for _, p := range svc.Ports {
		if p.Target > math.MaxUint16 {
			return errs.Errorf(
				ErrInvalidPort,
				"invalid container port %d for published port %q",
				p.Target,
				p.Published,
			)
		}

		targets = append(targets, uint16(p.Target))
	}

	return resolveNgrok(dockerExt.Ngrok, &s.Ngrok, targets, ngrokEnabled)
}

func (s *DockerService) resolveServiceImage(
	ctx context.Context,
	svc compose.Service,
	gitClient git.Client,
) error {
	if s.Skip {
		return nil
	}

	return resolveServiceImage(ctx, &s.Image, svc, s.Dir, gitClient)
}
