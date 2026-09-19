package helm

import (
	"context"

	"github.com/robgonnella/minienv/internal/compose"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/resolver"
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

// InitProject exposes the half of Init that needs neither a compose file on
// disk nor a cluster. Init itself loads the project from a Source and then
// builds a helm action config against a live one, so this is the only way to
// hand a spec-built project to the deployer and populate h.services.
func (h *Helm) InitProject(
	ctx context.Context,
	project compose.Project,
	dirs compose.ServiceDirs,
	raw compose.RawServices,
) error {
	return h.initProject(ctx, project, dirs, raw)
}

// SetProject caches the project without resolving any extension. The walk's
// fixtures are deliberately unresolvable, so they cannot go through
// InitProject.
func (h *Helm) SetProject(project compose.Project) {
	h.project = project
}

// ServiceExtension exposes what initProject resolved for one service. The
// per-service options only meet the extension inside initProject, so this is
// the only place to assert that they arrived.
func (h *Helm) ServiceExtension(name string) resolver.K8sService {
	return h.services[name]
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
	deploy func(ctx context.Context, svc compose.Service) error,
) error {
	return h.deployInDependencyOrder(ctx, deploy)
}

// DestroyInReverseDependencyOrder is the teardown counterpart.
func (h *Helm) DestroyInReverseDependencyOrder(
	ctx context.Context,
	uninstall func(ctx context.Context, svc compose.Service) error,
) error {
	return h.destroyInReverseDependencyOrder(ctx, uninstall)
}

// DeployService exposes the per-service step the walk dispatches to. Only its
// pre-helm branches — skip, and a failure resolving the extension — are
// reachable without a cluster.
func (h *Helm) DeployService(
	ctx context.Context,
	svc compose.Service,
) error {
	return h.deployService(ctx, svc)
}
