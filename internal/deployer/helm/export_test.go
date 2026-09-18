package helm

import (
	"context"

	"github.com/robgonnella/minienv/internal/config"
	helmaction "helm.sh/helm/v3/pkg/action"
	"k8s.io/client-go/kubernetes"
)

// SetActionConfig installs an in-process action config. Init only ever builds
// one against a live cluster, so this is the only way to reach helm actions
// from a test binary.
func (h *Helm) SetActionConfig(cfg *helmaction.Configuration) {
	h.actionConfig = cfg
}

// SetKubernetesClientSet installs a clientset directly. The action config
// otherwise derives one from a kubeconfig, so the namespace calls are
// unreachable from a test binary without this.
func (h *Helm) SetKubernetesClientSet(cs kubernetes.Interface) {
	h.clientset = cs
}

// CreateNamespaceIfNotExists exposes the namespace step Deploy runs before
// any release is touched.
func (h *Helm) CreateNamespaceIfNotExists(ctx context.Context) error {
	return h.createNamespaceIfNotExists(ctx)
}

// DestroyNamespace exposes the step Destroy runs last, and only when the
// extension opts in.
func (h *Helm) DestroyNamespace(ctx context.Context) error {
	return h.destroyNamespace(ctx)
}

// UninstallChart exposes the per-release teardown step that both Destroy and
// the ngrok-off branch of Deploy dispatch to.
func (h *Helm) UninstallChart(ctx context.Context, name string) error {
	return h.uninstallChart(ctx, name)
}

// InitProject exposes the half of Init that needs no cluster. Init itself
// builds a helm action config against a live one, so this is the only way to
// populate h.services in a test binary.
func (h *Helm) InitProject(
	ctx context.Context,
	project config.ComposeProject,
) error {
	return h.initProject(ctx, project)
}

// SetProject caches the project without resolving any extension. The walk's
// fixtures are deliberately unresolvable, so they cannot go through
// InitProject.
func (h *Helm) SetProject(project config.ComposeProject) {
	h.project = project
}

// ServicesToPublish exposes what initProject derived. Which services earn an
// endpoint, and in what order, is decided here rather than in the chart.
func (h *Helm) ServicesToPublish() []config.NgrokEndpointConfig {
	return h.servicesToPublish
}

// BuildAndPushServiceImages exposes the image-build pass. Deploy reaches it
// only after Init has built a cluster connection.
func (h *Helm) BuildAndPushServiceImages(ctx context.Context) error {
	return h.buildAndPushServiceImages(ctx)
}

// DeployInDependencyOrder exposes the walk with the per-service step injected.
// Deploy reaches the real step only after Init has built a cluster connection.
func (h *Helm) DeployInDependencyOrder(
	ctx context.Context,
	deploy func(ctx context.Context, svc config.ComposeService) error,
) error {
	return h.deployInDependencyOrder(ctx, deploy)
}

// DestroyInReverseDependencyOrder is the teardown counterpart.
func (h *Helm) DestroyInReverseDependencyOrder(
	ctx context.Context,
	uninstall func(ctx context.Context, svc config.ComposeService) error,
) error {
	return h.destroyInReverseDependencyOrder(ctx, uninstall)
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
