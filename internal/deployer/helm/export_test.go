package helm

import (
	"context"

	"github.com/robgonnella/minienv/internal/config"
)

// InitProject exposes the half of Init that needs no cluster. Init itself
// builds a helm action config against a live one, so this is the only way to
// populate h.services — which buildAndPushServiceImages and deployService both
// read — in a test binary.
func (h *Helm) InitProject(
	ctx context.Context,
	project config.ComposeProject,
) error {
	return h.initProject(ctx, project)
}

// SetProject caches the project without resolving any extension. The dependency
// walk only reads h.project, and its fixtures are bare services carrying nothing
// but a name and depends_on — deliberately unresolvable — so those specs need
// this rather than InitProject.
func (h *Helm) SetProject(project config.ComposeProject) {
	h.project = project
}

// ServicesToPublish exposes the endpoints initProject derived from the compose
// project. Which services earn one — and in what order — is a decision made
// here, not in the chart, so specs assert on it directly rather than digging it
// back out of rendered ngrok values.
func (h *Helm) ServicesToPublish() []config.NgrokEndpointConfig {
	return h.servicesToPublish
}

// BuildAndPushServiceImages exposes the image-build pass to the external test
// package. Deploy only reaches it after Init has built a real action client
// and a cluster connection, so this is the only way to assert on the
// ComposeProject -> []image.ServiceProperties translation.
func (h *Helm) BuildAndPushServiceImages(ctx context.Context) error {
	return h.buildAndPushServiceImages(ctx)
}

// DeployInDependencyOrder exposes the depends_on-ordered walk with the
// per-service step injected. Deploy only reaches the real step after Init has
// built a cluster connection, so this is the only way to assert on ordering,
// concurrency and abort-on-first-error.
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
