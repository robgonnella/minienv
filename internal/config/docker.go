package config

import (
	"context"
	"math"

	"github.com/go-viper/mapstructure/v2"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/rs/zerolog/log"
)

// XMiniEnvDocker uses docker and a specified transport mechanism to deploy
// modified docker-compose config on a remote server.
type XMiniEnvDocker struct {
	// The namespace for this deployment to ensure there are no conflicts
	// with other user minienvs on the same host
	Namespace string `json:"namespace" mapstructure:"namespace"`
	// The transport mechanism to use for communication with the remote server
	Transport XMiniEnvDockerTransport `json:"transport" mapstructure:"transport"`
	// Ngrok configuration for exposing services publicly
	Ngrok *NgrokTopLevel `json:"ngrok,omitzero" mapstructure:"ngrok,omitzero"`
}

// XMiniEnvSSH holds the fields required for deploying to any SSH-enabled
// remote server.
type XMiniEnvSSH struct {
	// The target host for the SSH connection
	Host string `json:"host" mapstructure:"host"`
	// The user for the SSH connection
	User string `json:"user" mapstructure:"user"`
	// The port for the SSH connection
	Port uint16 `json:"port" mapstructure:"port"`
	// The RSA private key identity file for the SSH connection
	Identity string `json:"identity" mapstructure:"identity"`
}

// XMiniEnvDockerTransport holds the transport mechanism for copying files
// and running commands on a remote server. Only one transport can be defined
// at a time.
type XMiniEnvDockerTransport struct {
	// Copies files and runs commands on the remote server over SSH
	SSH *XMiniEnvSSH `json:"ssh,omitempty" mapstructure:"ssh,omitempty"`
}

// ConfigFields returns the available "json" config fields for this extension.
func (x *XMiniEnvDockerTransport) ConfigFields() []string {
	return []string{"ssh"}
}

// XMiniEnvDockerService is the service level configuration controlling docker
// deployment properties.
type XMiniEnvDockerService struct {
	XMiniEnvCommonService `mapstructure:",squash"`

	// The image for this deployment. Will try to use compose service image if not set
	Image ServiceImage `json:"image,omitzero" mapstructure:"image,omitzero"`
}

func NewXMiniEnvDockerService(
	ctx context.Context,
	svc ComposeService,
	dockerExt XMiniEnvDocker,
	gitClient git.Client,
	ngrokEnabled bool,
) (*XMiniEnvDockerService, error) {
	log.Info().Str("service", svc.Name).Msg("loading service extension")

	rawSvcExt, ok := svc.Extensions[DockerServiceExtension]
	if !ok {
		rawSvcExt = map[string]any{}
	}

	svcExt := &XMiniEnvDockerService{}
	if err := mapstructure.Decode(rawSvcExt, svcExt); err != nil {
		return nil, errs.Errorf(
			ErrExtensionDecode,
			"failed to parse x-minienv-ssh-service extension: %w",
			err,
		)
	}

	if err := svcExt.resolve(
		ctx,
		dockerExt,
		rawSvcExt,
		svc,
		gitClient,
		ngrokEnabled,
	); err != nil {
		return nil, err
	}

	log.
		Info().
		Msgf(
			"resolved %s extension: %+v",
			DockerServiceExtension,
			svcExt,
		)

	return svcExt, nil
}

func (s *XMiniEnvDockerService) resolve(
	ctx context.Context,
	dockerExt XMiniEnvDocker,
	rawSvcExt any,
	svc ComposeService,
	gitClient git.Client,
	ngrokEnabled bool,
) error {
	if err := s.resolveCommonProperties(rawSvcExt); err != nil {
		return err
	}

	if err := s.resolveServiceImage(ctx, svc, gitClient); err != nil {
		return err
	}

	return s.resolveNgrok(svc, dockerExt, ngrokEnabled)
}

func (s *XMiniEnvDockerService) resolveCommonProperties(
	rawSvcExt any,
) error {
	common := XMiniEnvCommonService{}
	if err := mapstructure.Decode(rawSvcExt, &common); err != nil {
		return errs.Errorf(
			ErrExtensionDecode,
			"failed to parse common service properties: %w",
			err,
		)
	}

	s.XMiniEnvCommonService = common

	return nil
}

func (s *XMiniEnvDockerService) resolveNgrok(
	svc ComposeService,
	dockerExt XMiniEnvDocker,
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

func (s *XMiniEnvDockerService) resolveServiceImage(
	ctx context.Context,
	svc ComposeService,
	gitClient git.Client,
) error {
	return resolveServiceImage(ctx, &s.Image, svc, gitClient)
}
