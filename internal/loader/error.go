// Package loader turns docker-compose configuration on disk into a runnable
// Core, resolving the minienv extensions and picking the active deployer.
package loader

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrProjectOptions           errs.Kind = "loader.project_options"
	ErrProjectLoad              errs.Kind = "loader.project_load"
	ErrNoExtension              errs.Kind = "loader.no_extension"
	ErrExtensionDecode          errs.Kind = "loader.extension_decode"
	ErrMultipleDeployers        errs.Kind = "loader.multiple_deployers"
	ErrNoActiveDeployer         errs.Kind = "loader.no_active_deployer"
	ErrMultipleDockerTransports errs.Kind = "loader.multiple_docker_transports"
	ErrNoActiveDockerTransport  errs.Kind = "loader.no_active_docker_transport"
	ErrIncludeModel             errs.Kind = "loader.include_model"
	ErrIncludeDecode            errs.Kind = "loader.include_decode"
	ErrIncludeCycle             errs.Kind = "loader.include_cycle"
	ErrServiceDirMissing        errs.Kind = "loader.service_dir_missing"
)
