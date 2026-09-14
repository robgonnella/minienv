package core

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrDeployerInit errs.Kind = "core.deployer_init"
	ErrDeploy       errs.Kind = "core.deploy"
	ErrDestroy      errs.Kind = "core.destroy"
)
