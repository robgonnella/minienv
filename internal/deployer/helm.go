package deployer

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/image"
	"github.com/robgonnella/minienv/internal/os"
	"github.com/rs/zerolog/log"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmloader "helm.sh/helm/v3/pkg/chart/loader"
	helmcli "helm.sh/helm/v3/pkg/cli"
	k8sv1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Helm struct {
	ext            *config.XMiniEnv
	actionConfig   *helmaction.Configuration
	docker         *image.Docker
	commander      os.Commander
	ngrokAuthToken string
	dryRun         bool
}

func NewHelm(
	ext *config.XMiniEnv,
	commander os.Commander,
	ngrokAuthToken string,
	dryRun bool,
) *Helm {
	return &Helm{
		ext:            ext,
		actionConfig:   nil,
		docker:         image.NewDocker(commander, dryRun),
		commander:      commander,
		ngrokAuthToken: ngrokAuthToken,
		dryRun:         dryRun,
	}
}

func (h *Helm) Active() bool {
	return h.ext.K8s.Context != "" && h.ext.K8s.Namespace != ""
}

func (h *Helm) ConfigField() string {
	return "k8s"
}

func (h *Helm) String() string {
	return "Helm"
}

func (h *Helm) Init(project *config.ComposeProject) error {
	if !h.Active() {
		return nil
	}

	settings := helmcli.New()
	settings.SetNamespace(h.ext.K8s.Namespace)
	settings.KubeContext = h.ext.K8s.Context

	actionConfig := new(helmaction.Configuration)

	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		settings.Namespace(),
		config.HELM_DRIVER,
		log.Printf,
	); err != nil {
		return err
	}

	h.actionConfig = actionConfig
	return nil
}

func (h *Helm) Deploy(project *config.ComposeProject) error {
	if !h.Active() {
		return nil
	}

	if err := h.buildAndPushServiceImages(project); err != nil {
		return err
	}

	if err := h.createNamespaceIfNotExists(); err != nil {
		return err
	}

	for _, svc := range project.Services {
		svcExt, err := config.NewXMiniEnvK8sService(h.ext, svc, h.commander)
		if err != nil {
			return err
		}

		if svcExt.Skip {
			log.
				Warn().
				Str("service", svc.Name).
				Msg("detected skip: omitting service from deployment")
			continue
		}

		if err := h.upgradeOrInstallService(svcExt, svc); err != nil {
			return err
		}
	}

	return nil
}

func (h *Helm) Destroy(project *config.ComposeProject) error {
	if !h.Active() {
		return nil
	}

	for name, svc := range project.Services {
		if err := h.uninstallChart(svc); err != nil {
			return Errorf("failed to destroy service %s: %s", name, err)
		}
	}

	return nil
}

func (h *Helm) buildAndPushServiceImages(project *config.ComposeProject) error {
	dockerServices := []image.DockerService{}
	for _, svc := range project.Services {
		svcExt, err := config.NewXMiniEnvK8sService(h.ext, svc, h.commander)
		if err != nil {
			return err
		}
		if svcExt.Skip {
			log.Warn().Str("service", svc.Name).Msg("detected skip: skipping")
			continue
		}

		platforms := []string{"linux/amd64"}

		if len(svcExt.Image.Platforms) != 0 {
			platforms = svcExt.Image.Platforms
		}

		if svc.Build != nil {
			dockerServices = append(dockerServices, image.DockerService{
				Name:       svc.Name,
				Registry:   svcExt.Image.Repository,
				Tag:        svcExt.Image.Tag,
				Context:    svc.Build.Context,
				Dockerfile: svc.Build.Dockerfile,
				Platforms:  platforms,
				Args:       svc.Build.Args.ToMapping(),
			})
		}
	}

	if len(dockerServices) > 0 {
		return h.docker.BuildAndPush(dockerServices)
	}

	return nil
}

func (h *Helm) upgradeOrInstallService(
	svcExt *config.XMiniEnvK8sService,
	svc config.ComposeService,
) error {
	exists := h.serviceReleaseExists(svc.Name)

	if exists {
		log.Info().Str("release", svc.Name).Msg("upgrading release")
		return h.upgradeChart(svcExt, svc)
	} else {
		log.Info().Str("release", svc.Name).Msg("installing release")
		return h.installChart(svcExt, svc)
	}
}

func (h *Helm) installChart(
	svcExt *config.XMiniEnvK8sService,
	svc config.ComposeService,
) error {
	values, err := svcExt.ToValuesMap()
	if err != nil {
		return err
	}

	timeout := svcExt.DeploymentTimeout
	if timeout == "" {
		timeout = config.HELM_DEFAULT_DEPLOYMENT_TIMEOUT
	}

	parsedTimeout, err := time.ParseDuration(timeout)
	if err != nil {
		return Errorf("invalid deploymentTimeout configuration: %s", err)
	}

	client := helmaction.NewInstall(h.actionConfig)
	client.ReleaseName = svc.Name
	client.Namespace = h.ext.K8s.Namespace
	client.CreateNamespace = false
	client.Wait = true
	client.Atomic = true
	client.Wait = true
	client.DryRun = h.dryRun
	client.Timeout = parsedTimeout

	chart, err := h.loadChart(svc.Name, h.ngrokAuthToken)
	if err != nil {
		return Errorf("failed to load in-memory chart: %s", err)
	}

	if _, err := client.Run(chart, values); err != nil {
		return Errorf("failed to install service chart %s: %s", svc.Name, err)
	}

	log.
		Info().
		Str("context", h.ext.K8s.Context).
		Str("namespace", h.ext.K8s.Namespace).
		Str("service", svc.Name).
		Msg("successfully installed service chart")

	return nil
}

func (h *Helm) upgradeChart(
	svcExt *config.XMiniEnvK8sService,
	svc config.ComposeService,
) error {
	values, err := svcExt.ToValuesMap()
	if err != nil {
		return err
	}

	timeout := svcExt.DeploymentTimeout
	if timeout == "" {
		timeout = config.HELM_DEFAULT_DEPLOYMENT_TIMEOUT
	}

	parsedTimeout, err := time.ParseDuration(timeout)
	if err != nil {
		return Errorf("invalid deploymentTimeout configuration: %s", err)
	}

	client := helmaction.NewUpgrade(h.actionConfig)
	client.Namespace = h.ext.K8s.Namespace
	client.Atomic = true
	client.Wait = true
	client.CleanupOnFail = true
	client.DryRun = h.dryRun
	client.Timeout = parsedTimeout

	chart, err := h.loadChart(svc.Name, h.ngrokAuthToken)
	if err != nil {
		return Errorf("failed to load in-memory chart: %s", err)
	}

	if _, err := client.Run(svc.Name, chart, values); err != nil {
		return Errorf("failed to upgrade service chart %s: %s", svc.Name, err)
	}

	log.
		Info().
		Str("context", h.ext.K8s.Context).
		Str("namespace", h.ext.K8s.Namespace).
		Str("service", svc.Name).
		Msg("successfully upgraded service chart")

	return nil
}

func (h *Helm) uninstallChart(svc config.ComposeService) error {
	client := helmaction.NewUninstall(h.actionConfig)
	client.Wait = true
	client.IgnoreNotFound = true
	client.DryRun = h.dryRun

	response, err := client.Run(svc.Name)
	if err != nil {
		return err
	}

	if response == nil || response.Release == nil {
		log.Info().Str("service", svc.Name).Msg("no release found for service")
		return nil
	}

	log.
		Info().
		Str("context", h.ext.K8s.Context).
		Str("service", response.Release.Name).
		Str("namespace", response.Release.Namespace).
		Msg("successfully uninstalled service")

	return nil
}

func (h *Helm) createNamespaceIfNotExists() error {
	clientset, err := h.actionConfig.KubernetesClientSet()
	if err != nil {
		return err
	}

	create := false

	_, err = clientset.
		CoreV1().
		Namespaces().
		Get(context.TODO(), h.ext.K8s.Namespace, k8smetav1.GetOptions{})

	if err != nil && k8s_errors.IsNotFound(err) {
		create = true
	} else if err != nil {
		return err
	}

	if !create {
		return nil
	}

	_, err = clientset.
		CoreV1().
		Namespaces().
		Create(
			context.TODO(),
			&k8sv1.Namespace{Name: h.ext.K8s.Namespace},
			k8smetav1.CreateOptions{},
		)

	return err
}

func (h *Helm) serviceReleaseExists(name string) bool {
	client := helmaction.NewGet(h.actionConfig)
	release, _ := client.Run(name)
	return release != nil
}

func (h *Helm) loadChart(
	svcName string,
	ngrokAuthToken string,
) (*helmchart.Chart, error) {
	files := []*helmloader.BufferedFile{
		{
			Name: "Chart.yaml",
			Data: []byte(h.getHelmChartYamlTxt(svcName)),
		},
		{
			Name: "values.yaml",
			Data: []byte(h.getHelmChartValuesTxt()),
		},
		{
			Name: "templates/_helpers.tpl",
			Data: []byte(h.getHelmHelpersTxt(svcName)),
		},
		{
			Name: "templates/deployment.yaml",
			Data: []byte(h.getHelmDeploymentTxt(svcName)),
		},
		{
			Name: "templates/service.yaml",
			Data: []byte(h.getHelmServiceTxt(svcName)),
		},
		{
			Name: "templates/serviceaccount.yaml",
			Data: []byte(h.getHelmServiceAccountTxt(svcName)),
		},
		{
			Name: "templates/configmap.yaml",
			Data: []byte(h.getHelmConfigMapTxt(svcName)),
		},
		{
			Name: "templates/secret.yaml",
			Data: []byte(h.getHelmSecretTxt(ngrokAuthToken)),
		},
	}

	return helmloader.LoadFiles(files)
}

func (h *Helm) getHelmChartYamlTxt(svcName string) string {
	return fmt.Sprintf(`
type: application
name: %[1]s
version: 0.1.0
appVersion: 0.1.0
description: A chart for deploying %[1]s service
`, svcName)
}

func (h *Helm) getHelmChartValuesTxt() string {
	return `
replicaCount: 1

image:
  repository: ""
  pullPolicy: IfNotPresent
  tag: ""

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""

env: {}

ngrok:
  enabled: false
  image: ""
  configMapName: ""
  configKey: ""
  configVolMountPath: ""
  secretName: ""
  port: ""
  url: ""
  trafficPolicy: ""

serviceAccount:
  create: true
  automount: true
  annotations: {}
  name: ""

podAnnotations: {}
podLabels: {}
podSecurityContext: {}
securityContext: {}

service:
  create: true
  type: ClusterIP
  ports: []

resources: {}

startupProbe: {}
livenessProbe: {}
readinessProbe: {}

volumes: []
volumeMounts: []
nodeSelector: {}
tolerations: []
affinity: {}
`
}

func (h *Helm) getHelmHelpersTxt(svcName string) string {
	return fmt.Sprintf(`
{{/*
Expand the name of the chart.
*/}}
{{- define "%[1]s.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "%[1]s.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%%s-%%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "%[1]s.chart" -}}
{{- printf "%%s-%%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "%[1]s.labels" -}}
helm.sh/chart: {{ include "%[1]s.chart" . }}
{{ include "%[1]s.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "%[1]s.selectorLabels" -}}
app.kubernetes.io/name: {{ include "%[1]s.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "%[1]s.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "%[1]s.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
`, svcName)
}

func (h *Helm) getHelmConfigMapTxt(svcName string) string {
	return fmt.Sprintf(`
{{- if .Values.ngrok.enabled -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Values.ngrok.configMapName }}
data:
  {{ .Values.ngrok.configKey }}: |
    version: 3
    endpoints:
      - name: {{ include "%[1]s.fullname" . }}
        url: {{ .Values.ngrok.url }}
        description: "endpoint for %[1]s"
        upstream:
          url: {{ include "%[1]s.fullname" . }}:{{ .Values.ngrok.port }}
        traffic_policy:
          {{ .Values.ngrok.trafficPolicy }}
{{- end -}}
`,
		svcName,
	)
}

func (h *Helm) getHelmSecretTxt(authToken string) string {
	return fmt.Sprintf(`
{{- if .Values.ngrok.enabled -}}
apiVersion: v1
kind: Secret
metadata:
  name: {{ .Values.ngrok.secretName }}
data:
  NGROK_AUTHTOKEN: %s
{{- end -}}
`,
		base64.StdEncoding.EncodeToString([]byte(authToken)),
	)
}

func (h *Helm) getHelmDeploymentTxt(svcName string) string {
	return fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "%[1]s.fullname" . }}
  labels:
    {{- include "%[1]s.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "%[1]s.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      {{- with .Values.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "%[1]s.labels" . | nindent 8 }}
        {{- with .Values.podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "%[1]s.serviceAccountName" . }}
      {{- with .Values.podSecurityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: {{ .Chart.Name }}
          {{- with .Values.securityContext }}
          securityContext:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            {{- range .Values.service.ports }}
            - name: {{ .containerPortName }}
              containerPort: {{ .containerPort }}
              protocol: {{ .protocol }}
            {{- end }}
          {{- if .Values.env }}
          env:
            {{- range $k, $v := .Values.env }}
            - name: {{ $k }}
              value: {{ $v }}
            {{- end }}
          {{- end }}
          {{- with .Values.startupProbe }}
          startupProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.livenessProbe }}
          livenessProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.readinessProbe }}
          readinessProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.volumeMounts }}
          volumeMounts:
            {{- toYaml . | nindent 12 }}
          {{- end }}
        {{- if .Values.ngrok.enabled }}
        - name: ngrok
          image: {{ .Values.ngrok.image }}
          imagePullPolicy: IfNotPresent
          command:
            - ngrok
            - start
            - {{ include "%[1]s.fullname" . }}
            - --log=stdout
          envFrom:
            - secretRef:
                name: {{ .Values.ngrok.secretName }}
          volumeMounts:
            - name: {{ .Values.ngrok.configMapName }}
              mountPath: {{ .Values.ngrok.configVolMountPath }}
        {{- end }}
      {{- with .Values.volumes }}
      volumes:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
`,
		svcName,
	)
}

func (h *Helm) getHelmServiceTxt(svcName string) string {
	return fmt.Sprintf(`
{{- if .Values.service.create -}}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "%[1]s.fullname" . }}
  labels:
    {{- include "%[1]s.labels" . | nindent 4 }}
spec:
  type: {{ .Values.service.type }}
  ports:
    {{- range .Values.service.ports }}
    - name: {{ .servicePortName }}
      port: {{ .servicePort }}
      targetPort: {{ .containerPortName }}
      protocol: {{ .protocol }}
    {{- end }}
  selector:
    {{- include "%[1]s.selectorLabels" . | nindent 4 }}
{{- end -}}
`, svcName)
}

func (h *Helm) getHelmServiceAccountTxt(svcName string) string {
	return fmt.Sprintf(`
{{- if .Values.serviceAccount.create -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "%[1]s.serviceAccountName" . }}
  labels:
    {{- include "%[1]s.labels" . | nindent 4 }}
  {{- with .Values.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
automountServiceAccountToken: {{ .Values.serviceAccount.automount }}
{{- end -}}
`, svcName)
}
