package helm

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrInvalidExtension       errs.Kind = "helm.invalid_extension"
	ErrActionConfig           errs.Kind = "helm.action_config"
	ErrChartUninstall         errs.Kind = "helm.chart_uninstall"
	ErrChartDeploymentTimeout errs.Kind = "helm.chart_deployment_timeout"
	ErrChartLoad              errs.Kind = "helm.chart_load"
	ErrChartInstall           errs.Kind = "helm.chart_install"
	ErrChartUpgrade           errs.Kind = "helm.chart_upgrade"
	ErrComposeDependencyGraph errs.Kind = "helm.compose_dependency_graph"
	ErrK8sNamespace           errs.Kind = "helm.k8s_namespace"
	ErrMissingService         errs.Kind = "helm.missing_service"
	ErrManifestRead           errs.Kind = "helm.manifest_read"
	ErrConfigMapFileRead      errs.Kind = "helm.config_map_file_read"
	ErrConfigMapFileEncoding  errs.Kind = "helm.config_map_file_encoding"
)
