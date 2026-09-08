package helm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"slices"
	"strconv"

	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmloader "helm.sh/helm/v3/pkg/chart/loader"
)

// Fixed so Destroy can find the release without rebuilding the ngrok config.
const ngrokReleaseName = "ngrok"

// The agent reads ngrok.yml only at startup, and the pod template is otherwise
// constant, so this is what makes helm replace the pod when the config changes.
func ngrokConfigChecksum(published []NgrokConfig) string {
	sum := sha256.New()

	for _, svc := range published {
		writeChecksumParts(
			sum,
			svc.Namespace,
			svc.EndpointName,
			svc.ServiceName,
			svc.Url,
			strconv.Itoa(int(svc.Port)),
			svc.TrafficPolicy,
		)
	}

	writeChecksumParts(
		sum,
		config.NGROK_CONFIG_MAP_NAME,
		config.NGROK_CONFIG_KEY,
		HELM_NGROK_CONFIG_MAP_TMPL,
	)

	return hex.EncodeToString(sum.Sum(nil))
}

// Rotating the auth token otherwise leaves the agent on the old credential.
func ngrokSecretChecksum(authToken string) string {
	sum := sha256.New()

	writeChecksumParts(
		sum,
		config.NGROK_SECRET_NAME,
		helmNgrokSecretTmpl(authToken),
	)

	return hex.EncodeToString(sum.Sum(nil))
}

// Length-prefixed so different splits of the same bytes cannot hash alike.
func writeChecksumParts(sum hash.Hash, parts ...string) {
	for _, part := range parts {
		_, _ = fmt.Fprintf(sum, "%d:%s", len(part), part)
	}
}

// ChartBuilder assembles the in-memory charts this package installs. It holds
// the ngrok auth token because two of its outputs derive from it — the Secret
// baked into the chart and the checksum annotation that rolls the pod when the
// token rotates — and passing it twice let the pair disagree.
type ChartBuilder struct {
	ngrokAuthToken string
}

func NewChartBuilder(ngrokAuthToken string) *ChartBuilder {
	return &ChartBuilder{ngrokAuthToken: ngrokAuthToken}
}

// Chart picks the chart shape a service's deploymentType calls for.
func (b *ChartBuilder) Chart(
	svcName string,
	deploymentType config.K8sDeploymentType,
) (*helmchart.Chart, error) {
	if deploymentType == config.K8sJobDeploymentType {
		return b.JobChart(svcName)
	}

	return b.ServiceChart(svcName)
}

func (b *ChartBuilder) NgrokChart() (*helmchart.Chart, error) {
	files := b.ngrokFiles()

	chart, err := helmloader.LoadFiles(files)
	if err != nil {
		return nil, errs.Errorf(
			KindChartLoad,
			"failed to load in-memory ngrok chart: %w",
			err,
		)
	}

	return chart, nil
}

func (b *ChartBuilder) ServiceChart(
	svcName string,
) (*helmchart.Chart, error) {
	files := b.serviceFiles(svcName)

	chart, err := helmloader.LoadFiles(files)
	if err != nil {
		return nil, errs.Errorf(
			KindChartLoad,
			"failed to load in-memory chart for %s: %w",
			svcName,
			err,
		)
	}

	return chart, nil
}

func (b *ChartBuilder) JobChart(svcName string) (*helmchart.Chart, error) {
	files := b.jobFiles(svcName)

	chart, err := helmloader.LoadFiles(files)
	if err != nil {
		return nil, errs.Errorf(
			KindChartLoad,
			"failed to load in-memory chart for %s: %w",
			svcName,
			err,
		)
	}

	return chart, nil
}

func (b *ChartBuilder) NgrokValues(toPublish []NgrokConfig) map[string]any {
	if len(toPublish) == 0 {
		return nil
	}

	values := map[string]any{
		"image": map[string]any{
			"repository": config.NGROK_IMAGE_REPO,
			"tag":        config.NGROK_IMAGE_TAG,
		},
		"configMapName":      config.NGROK_CONFIG_MAP_NAME,
		"configKey":          config.NGROK_CONFIG_KEY,
		"configVolMountPath": config.NGROK_CONFIG_VOL_MOUNT_PATH,
		"secretName":         config.NGROK_SECRET_NAME,
		"command": []string{
			"ngrok",
			"start",
			"--all",
			"--log=stdout",
		},
		// RollingUpdate would overlap two agents claiming the same endpoints.
		"strategy": map[string]any{"type": "Recreate"},
	}

	endpoints := []map[string]any{}
	for _, svc := range toPublish {
		endpoints = append(endpoints, map[string]any{
			"endpointName":  svc.EndpointName,
			"namespace":     svc.Namespace,
			"serviceName":   svc.ServiceName,
			"url":           svc.Url,
			"port":          svc.Port,
			"trafficPolicy": svc.TrafficPolicy,
		})
	}
	values["endpoints"] = endpoints

	values["volumes"] = []map[string]any{
		{
			"name": config.NGROK_CONFIG_MAP_NAME,
			"configMap": map[string]any{
				"name": config.NGROK_CONFIG_MAP_NAME,
				"items": []map[string]any{
					{
						"key":  config.NGROK_CONFIG_KEY,
						"path": config.NGROK_CONFIG_KEY,
					},
				},
			},
		},
	}

	values["volumeMounts"] = []map[string]any{
		{
			"name":      config.NGROK_CONFIG_MAP_NAME,
			"mountPath": config.NGROK_CONFIG_VOL_MOUNT_PATH,
		},
	}

	values["envFrom"] = []map[string]any{
		{
			"secretRef": map[string]any{
				"name": config.NGROK_SECRET_NAME,
			},
		},
	}

	values["podAnnotations"] = map[string]string{
		"checksum/config": ngrokConfigChecksum(toPublish),
		"checksum/secret": ngrokSecretChecksum(b.ngrokAuthToken),
	}

	return values
}

func (b *ChartBuilder) commonFiles(svcName string) []*helmloader.BufferedFile {
	return []*helmloader.BufferedFile{
		{
			Name: "Chart.yaml",
			Data: []byte(helmChartYamlTmpl(svcName)),
		},
		{
			Name: "templates/_helpers.tpl",
			Data: []byte(HELM_HELPERS_TMPL),
		},
	}
}

func (b *ChartBuilder) serviceFiles(svcName string) []*helmloader.BufferedFile {
	files := b.commonFiles(svcName)
	return slices.Concat(files, []*helmloader.BufferedFile{
		{
			Name: "values.yaml",
			Data: []byte(HELM_DEPLOYMENT_VALUES_TMPL),
		},
		{
			Name: "templates/deployment.yaml",
			Data: []byte(HELM_DEPLOYMENT_TMPL),
		},
		{
			Name: "templates/service.yaml",
			Data: []byte(HELM_SERVICE_TMPL),
		},
		{
			Name: "templates/serviceaccount.yaml",
			Data: []byte(HELM_SERVICE_ACCOUNT_TMPL),
		},
	})
}

func (b *ChartBuilder) jobFiles(svcName string) []*helmloader.BufferedFile {
	files := b.commonFiles(svcName)
	return slices.Concat(files, []*helmloader.BufferedFile{
		{
			Name: "values.yaml",
			Data: []byte(HELM_JOB_VALUES_TMPL),
		},
		{
			Name: "templates/serviceaccount.yaml",
			Data: []byte(HELM_SERVICE_ACCOUNT_TMPL),
		},
		{
			Name: "templates/job.yaml",
			Data: []byte(HELM_JOB_TMPL),
		},
	})
}

func (b *ChartBuilder) ngrokFiles() []*helmloader.BufferedFile {
	files := b.commonFiles(ngrokReleaseName)
	return slices.Concat(files, []*helmloader.BufferedFile{
		{
			Name: "values.yaml",
			Data: []byte(HELM_NGROK_VALUES_TMPL),
		},
		{
			Name: "templates/deployment.yaml",
			Data: []byte(HELM_DEPLOYMENT_TMPL),
		},
		{
			Name: "templates/serviceaccount.yaml",
			Data: []byte(HELM_SERVICE_ACCOUNT_TMPL),
		},
		{
			Name: "templates/configmap.yaml",
			Data: []byte(HELM_NGROK_CONFIG_MAP_TMPL),
		},
		{
			Name: "templates/secret.yaml",
			Data: []byte(helmNgrokSecretTmpl(b.ngrokAuthToken)),
		},
	})
}
