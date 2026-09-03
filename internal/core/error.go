package core

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	KindDeployerInit errs.Kind = "core.deployer_init"
	KindDeploy       errs.Kind = "core.deploy"
	KindDestroy      errs.Kind = "core.destroy"
)
