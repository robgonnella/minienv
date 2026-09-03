package deployer

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	KindDestroyService    errs.Kind = "deployer.destroy_service"
	KindDeploymentTimeout errs.Kind = "deployer.deployment_timeout"
	KindChartLoad         errs.Kind = "deployer.chart_load"
	KindInstall           errs.Kind = "deployer.install"
	KindUpgrade           errs.Kind = "deployer.upgrade"
)
