package core

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	ErrDeployerInit errs.Kind = "core.deployer_init"
	ErrDeploy       errs.Kind = "core.deploy"
	ErrDestroy      errs.Kind = "core.destroy"
)
