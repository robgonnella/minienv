package helm

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	KindChartUninstall         errs.Kind = "helm.chart_uninstall"
	KindChartDeploymentTimeout errs.Kind = "helm.chart_deployment_timeout"
	KindChartLoad              errs.Kind = "helm.chart_load"
	KindChartInstall           errs.Kind = "helm.chart_install"
	KindChartUpgrade           errs.Kind = "helm.chart_upgrade"
	KindComposeDependencyGraph errs.Kind = "helm.compose_dependency_graph"
	KindMissingService         errs.Kind = "helm.missing_service"
)
