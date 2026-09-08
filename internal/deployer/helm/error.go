package helm

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	ErrActionConfig           errs.Kind = "helm.action_config"
	ErrChartUninstall         errs.Kind = "helm.chart_uninstall"
	ErrChartDeploymentTimeout errs.Kind = "helm.chart_deployment_timeout"
	ErrChartLoad              errs.Kind = "helm.chart_load"
	ErrChartInstall           errs.Kind = "helm.chart_install"
	ErrChartUpgrade           errs.Kind = "helm.chart_upgrade"
	ErrComposeDependencyGraph errs.Kind = "helm.compose_dependency_graph"
	ErrK8sNamespace           errs.Kind = "helm.k8s_namespace"
	ErrMissingService         errs.Kind = "helm.missing_service"
)
