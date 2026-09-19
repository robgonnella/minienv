// Package loader reads the x-minienv extension out of the compose project,
// picks the active deployer and hands it a runnable Core.
package loader

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrNoExtension              errs.Kind = "loader.no_extension"
	ErrExtensionDecode          errs.Kind = "loader.extension_decode"
	ErrMultipleDeployers        errs.Kind = "loader.multiple_deployers"
	ErrNoActiveDeployer         errs.Kind = "loader.no_active_deployer"
	ErrMultipleDockerTransports errs.Kind = "loader.multiple_docker_transports"
	ErrNoActiveDockerTransport  errs.Kind = "loader.no_active_docker_transport"
)
