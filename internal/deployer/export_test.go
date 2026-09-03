package deployer

import (
	"github.com/robgonnella/minienv/internal/config"
	helmchart "helm.sh/helm/v3/pkg/chart"
)

// LoadChart exposes the in-memory chart assembly to the external test package.
// The chart is built from Go string templates rather than files on disk, so
// this is the only way to assert on what actually gets installed.
func (h *Helm) LoadChart(
	svcName string,
	ngrokAuthToken string,
) (*helmchart.Chart, error) {
	return h.loadChart(svcName, ngrokAuthToken)
}

// BuildAndPushServiceImages exposes the image-build pass to the external test
// package. Deploy only reaches it after Init has built a real action client
// and a cluster connection, so this is the only way to assert on the
// ComposeProject -> []image.ServiceProperties translation.
func (h *Helm) BuildAndPushServiceImages(
	project *config.ComposeProject,
) error {
	return h.buildAndPushServiceImages(project)
}
