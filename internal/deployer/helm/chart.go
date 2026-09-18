// Package helm deploys a compose project as a set of in-memory helm charts.
package helm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmloader "helm.sh/helm/v3/pkg/chart/loader"
)

// Fixed so Destroy can find the release without rebuilding the ngrok config.
const ngrokReleaseName = "ngrok"

// Chart file names and value keys the templates agree on.
const (
	nameKey            = "name"
	valuesFile         = "values.yaml"
	templatesDir       = "templates"
	serviceAccountFile = "templates/serviceaccount.yaml"
	deploymentFile     = "templates/deployment.yaml"
	configMapFile      = "templates/configmap.yaml"
)

// Manifests nest here so one named deployment.yaml cannot replace the
// generated template.
const manifestsDir = "manifests"

const configMapFilesDir = "files/configmap"

// The agent reads ngrok.yml only at startup, and the pod template is otherwise
// constant, so this is what makes helm replace the pod when the config changes.
func ngrokConfigChecksum(published []config.NgrokEndpointConfig) string {
	sum := sha256.New()

	for _, svc := range published {
		writeChecksumParts(
			sum,
			svc.Namespace,
			svc.EndpointName,
			svc.ServiceName,
			svc.URL,
			strconv.Itoa(int(svc.Port)),
			svc.TrafficPolicy,
		)
	}

	writeChecksumParts(
		sum,
		config.NgrokConfigMapName,
		config.NgrokConfigKey,
		ngrokConfigMapTmpl,
	)

	return hex.EncodeToString(sum.Sum(nil))
}

// Rotating the auth token otherwise leaves the agent on the old credential.
func ngrokSecretChecksum(authToken string) string {
	sum := sha256.New()

	writeChecksumParts(
		sum,
		config.NgrokSecretName,
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

// ChartBuilder holds the ngrok auth token so the Secret baked into the chart
// and the checksum that rolls the pod cannot disagree on it.
type ChartBuilder struct {
	ngrokAuthToken string
}

func NewChartBuilder(ngrokAuthToken string) *ChartBuilder {
	return &ChartBuilder{ngrokAuthToken: ngrokAuthToken}
}

func (b *ChartBuilder) Build(
	svcName string,
	deploymentType config.K8sDeploymentType,
	manifests []string,
	configMapFrom []string,
) (*helmchart.Chart, error) {
	if deploymentType == config.K8sJobDeploymentType {
		return b.jobChart(svcName, manifests, configMapFrom)
	}

	return b.serviceChart(svcName, manifests, configMapFrom)
}

func (b *ChartBuilder) BuildNgrok() (*helmchart.Chart, error) {
	files := b.ngrokFiles()

	chart, err := helmloader.LoadFiles(files)
	if err != nil {
		return nil, errs.Errorf(
			ErrChartLoad,
			"failed to load in-memory ngrok chart: %w",
			err,
		)
	}

	return chart, nil
}

func (b *ChartBuilder) NgrokValues(
	toPublish []config.NgrokEndpointConfig,
) map[string]any {
	if len(toPublish) == 0 {
		return nil
	}

	values := map[string]any{
		"image": map[string]any{
			"repository": config.NgrokImageRepo,
			"tag":        config.NgrokImageTag,
		},
		"configMapName":      config.NgrokConfigMapName,
		"configKey":          config.NgrokConfigKey,
		"configVolMountPath": config.NgrokConfigVolMountPath,
		"secretName":         config.NgrokSecretName,
		"command": []string{
			"ngrok",
			"start",
			"--all",
			"--log=stdout",
		},
		// RollingUpdate would overlap two agents claiming the same endpoints.
		"strategy": map[string]any{"type": "Recreate"},
	}

	values["endpoints"] = ngrokEndpointValues(toPublish)

	values["volumes"] = ngrokVolumeValues()

	values["volumeMounts"] = []map[string]any{
		{
			nameKey:     config.NgrokConfigMapName,
			"mountPath": config.NgrokConfigVolMountPath,
		},
	}

	values["envFrom"] = []map[string]any{
		{
			"secretRef": map[string]any{
				nameKey: config.NgrokSecretName,
			},
		},
	}

	values["podAnnotations"] = map[string]string{
		"checksum/config": ngrokConfigChecksum(toPublish),
		"checksum/secret": ngrokSecretChecksum(b.ngrokAuthToken),
	}

	return values
}

func (b *ChartBuilder) serviceChart(
	svcName string,
	manifests []string,
	configMapFrom []string,
) (*helmchart.Chart, error) {
	files, err := b.serviceFiles(svcName, manifests, configMapFrom)
	if err != nil {
		return nil, err
	}

	chart, err := helmloader.LoadFiles(files)
	if err != nil {
		return nil, errs.Errorf(
			ErrChartLoad,
			"failed to load in-memory chart for %s: %w",
			svcName,
			err,
		)
	}

	return chart, nil
}

func (b *ChartBuilder) jobChart(
	svcName string,
	manifests []string,
	configMapFrom []string,
) (*helmchart.Chart, error) {
	files, err := b.jobFiles(svcName, manifests, configMapFrom)
	if err != nil {
		return nil, err
	}

	chart, err := helmloader.LoadFiles(files)
	if err != nil {
		return nil, errs.Errorf(
			ErrChartLoad,
			"failed to load in-memory chart for %s: %w",
			svcName,
			err,
		)
	}

	return chart, nil
}

func ngrokVolumeValues() []map[string]any {
	return []map[string]any{
		{
			nameKey: config.NgrokConfigMapName,
			"configMap": map[string]any{
				nameKey: config.NgrokConfigMapName,
				"items": []map[string]any{
					{
						"key":  config.NgrokConfigKey,
						"path": config.NgrokConfigKey,
					},
				},
			},
		},
	}
}

func ngrokEndpointValues(
	toPublish []config.NgrokEndpointConfig,
) []map[string]any {
	endpoints := make([]map[string]any, 0, len(toPublish))
	for _, svc := range toPublish {
		endpoints = append(endpoints, map[string]any{
			"endpointName":  svc.EndpointName,
			"namespace":     svc.Namespace,
			"serviceName":   svc.ServiceName,
			"url":           svc.URL,
			"port":          svc.Port,
			"trafficPolicy": svc.TrafficPolicy,
		})
	}

	return endpoints
}

func (b *ChartBuilder) commonFiles(svcName string) []*helmloader.BufferedFile {
	return []*helmloader.BufferedFile{
		{
			Name: "Chart.yaml",
			Data: []byte(helmChartYamlTmpl(svcName)),
		},
		{
			Name: "templates/_helpers.tpl",
			Data: []byte(helpersTmpl),
		},
	}
}

// Read here rather than while the extension resolves, so a file deleted since
// the last deploy cannot block a teardown.
func (b *ChartBuilder) loadManifests(
	manifests []string,
) ([]*helmloader.BufferedFile, error) {
	if len(manifests) == 0 {
		return nil, nil
	}

	files := make([]*helmloader.BufferedFile, 0, len(manifests))

	for _, p := range manifests {
		// #nosec G304 -- validated as a project-relative path in config.
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, errs.Errorf(
				ErrManifestRead,
				"failed to read manifest %s: %w",
				p,
				err,
			)
		}

		// path, not filepath: helm routes templates on a "/"-separated prefix.
		files = append(files, &helmloader.BufferedFile{
			Name: path.Join(templatesDir, manifestsDir, filepath.Base(p)),
			Data: data,
		})
	}

	return files, nil
}

func (b *ChartBuilder) loadConfigMapFiles(
	paths []string,
) ([]*helmloader.BufferedFile, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	files := make([]*helmloader.BufferedFile, 0, len(paths))

	for _, p := range paths {
		// #nosec G304 -- validated as a project-relative path in config.
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, errs.Errorf(
				ErrConfigMapFileRead,
				"failed to read configMapFrom file %s: %w",
				p,
				err,
			)
		}

		if !utf8.Valid(data) {
			return nil, errs.Errorf(
				ErrConfigMapFileEncoding,
				"configMapFrom file %s is not UTF-8 text",
				p,
			)
		}

		files = append(files, &helmloader.BufferedFile{
			Name: path.Join(configMapFilesDir, filepath.Base(p)),
			Data: data,
		})
	}

	return files, nil
}

func (b *ChartBuilder) serviceFiles(
	svcName string,
	manifests []string,
	configMapFrom []string,
) ([]*helmloader.BufferedFile, error) {
	manifestFiles, err := b.loadManifests(manifests)
	if err != nil {
		return nil, err
	}

	configMapFiles, err := b.loadConfigMapFiles(configMapFrom)
	if err != nil {
		return nil, err
	}

	files := b.commonFiles(svcName)

	return slices.Concat(files, []*helmloader.BufferedFile{
		{
			Name: valuesFile,
			Data: []byte(deploymentValuesTmpl),
		},
		{
			Name: deploymentFile,
			Data: []byte(deploymentTmpl),
		},
		{
			Name: "templates/service.yaml",
			Data: []byte(serviceTmpl),
		},
		{
			Name: serviceAccountFile,
			Data: []byte(serviceAccountTmpl),
		},
		{
			Name: configMapFile,
			Data: []byte(filesConfigMapTmpl),
		},
	}, manifestFiles, configMapFiles), nil
}

func (b *ChartBuilder) jobFiles(
	svcName string,
	manifests []string,
	configMapFrom []string,
) ([]*helmloader.BufferedFile, error) {
	manifestFiles, err := b.loadManifests(manifests)
	if err != nil {
		return nil, err
	}

	configMapFiles, err := b.loadConfigMapFiles(configMapFrom)
	if err != nil {
		return nil, err
	}

	files := b.commonFiles(svcName)

	return slices.Concat(files, []*helmloader.BufferedFile{
		{
			Name: valuesFile,
			Data: []byte(jobValuesTmpl),
		},
		{
			Name: serviceAccountFile,
			Data: []byte(serviceAccountTmpl),
		},
		{
			Name: "templates/job.yaml",
			Data: []byte(jobTmpl),
		},
		{
			Name: configMapFile,
			Data: []byte(filesConfigMapTmpl),
		},
	}, manifestFiles, configMapFiles), nil
}

func (b *ChartBuilder) ngrokFiles() []*helmloader.BufferedFile {
	files := b.commonFiles(ngrokReleaseName)

	return slices.Concat(files, []*helmloader.BufferedFile{
		{
			Name: valuesFile,
			Data: []byte(ngrokValuesTmpl),
		},
		{
			Name: deploymentFile,
			Data: []byte(deploymentTmpl),
		},
		{
			Name: serviceAccountFile,
			Data: []byte(serviceAccountTmpl),
		},
		{
			Name: configMapFile,
			Data: []byte(ngrokConfigMapTmpl),
		},
		{
			Name: "templates/secret.yaml",
			Data: []byte(helmNgrokSecretTmpl(b.ngrokAuthToken)),
		},
	})
}
