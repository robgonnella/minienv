package deployer

import (
	"context"

	"github.com/robgonnella/minienv/internal/config"
	helmchart "helm.sh/helm/v3/pkg/chart"
)

// LoadChart exposes the in-memory chart assembly to the external test package.
// The chart is built from Go string templates rather than files on disk, so
// this is the only way to assert on what actually gets installed.
func (h *Helm) LoadChart(
	svcName string,
	deploymentType config.K8sDeploymentType,
) (*helmchart.Chart, error) {
	return h.loadChart(svcName, deploymentType)
}

// InitProject exposes the half of Init that needs no cluster. Init itself
// builds a helm action config against a live one, so this is the only way to
// populate h.services — which buildAndPushServiceImages and deployService both
// read — in a test binary.
func (h *Helm) InitProject(project *config.ComposeProject) error {
	return h.initProject(project)
}

// SetProject caches the project without resolving any extension. The dependency
// walk only reads h.project, and its fixtures are bare services carrying nothing
// but a name and depends_on — deliberately unresolvable — so those specs need
// this rather than InitProject.
func (h *Helm) SetProject(project *config.ComposeProject) {
	h.project = project
}

// BuildAndPushServiceImages exposes the image-build pass to the external test
// package. Deploy only reaches it after Init has built a real action client
// and a cluster connection, so this is the only way to assert on the
// ComposeProject -> []image.ServiceProperties translation.
func (h *Helm) BuildAndPushServiceImages() error {
	return h.buildAndPushServiceImages()
}

// DeployInDependencyOrder exposes the depends_on-ordered walk with the
// per-service step injected. Deploy only reaches the real step after Init has
// built a cluster connection, so this is the only way to assert on ordering,
// concurrency and abort-on-first-error.
func (h *Helm) DeployInDependencyOrder(
	deploy func(ctx context.Context, svc config.ComposeService) error,
) error {
	return h.deployInDependencyOrder(deploy)
}

// DestroyInReverseDependencyOrder is the teardown counterpart.
func (h *Helm) DestroyInReverseDependencyOrder(
	uninstall func(ctx context.Context, svc config.ComposeService) error,
) error {
	return h.destroyInReverseDependencyOrder(uninstall)
}

// DeployService exposes the per-service step the walk dispatches to. Only its
// pre-helm branches — skip, and a failure resolving the extension — are
// reachable without a cluster.
func (h *Helm) DeployService(
	ctx context.Context,
	svc config.ComposeService,
) error {
	return h.deployService(ctx, svc)
}
