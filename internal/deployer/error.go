package deployer

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	KindChartUninstall         errs.Kind = "deployer.chart_uninstall"
	KindChartDeploymentTimeout errs.Kind = "deployer.chart_deployment_timeout"
	KindChartLoad              errs.Kind = "deployer.chart_load"
	KindChartInstall           errs.Kind = "deployer.chart_install"
	KindChartUpgrade           errs.Kind = "deployer.chart_upgrade"
	KindComposeDependencyGraph errs.Kind = "deployer.compose_dependency_graph"
	KindHelmMissingService     errs.Kind = "deployer.helm_missing_services"
)
