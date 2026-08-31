package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/rs/zerolog/log"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmloader "helm.sh/helm/v3/pkg/chart/loader"
	helmcli "helm.sh/helm/v3/pkg/cli"
	k8sv1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func loadChart(svcName string) (*helmchart.Chart, error) {
	files := []*helmloader.BufferedFile{
		{
			Name: "Chart.yaml",
			Data: []byte(getHelmChartYamlTxt(svcName)),
		},
		{
			Name: "values.yaml",
			Data: []byte(getHelmChartValuesTxt()),
		},
		{
			Name: "templates/_helpers.tpl",
			Data: []byte(getHelmHelpersTxt(svcName)),
		},
		{
			Name: "templates/deployment.yaml",
			Data: []byte(getHelmDeploymentTxt(svcName)),
		},
		{
			Name: "templates/service.yaml",
			Data: []byte(getHelmServiceTxt(svcName)),
		},
		{
			Name: "templates/serviceaccount.yaml",
			Data: []byte(getHelmServiceAccountTxt(svcName)),
		},
		{
			Name: "templates/configmap.yaml",
			Data: []byte(getHelmConfigMapTxt()),
		},
		{
			Name: "templates/secret.yaml",
			Data: []byte(getHelmSecretTxt()),
		},
	}

	return helmloader.LoadFiles(files)
}

func getHelmChartYamlTxt(svcName string) string {
	return fmt.Sprintf(`
type: application
name: %[1]s
version: 0.1.0
appVersion: 0.1.0
description: A chart for deploying %[1]s service
`, svcName)
}

func getHelmChartValuesTxt() string {
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
  port: ""
  on_http_request: ""

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

func getHelmHelpersTxt(svcName string) string {
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

func getHelmConfigMapTxt() string {
	return fmt.Sprintf(`
{{- if .Values.ngrok.enabled -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
data:
  %s: |
    {{ .Values.ngrok.trafficPolicy | toYaml | nindent 4 }}
{{- end -}}
`,
		config.NGROK_CONFIG_MAP_NAME,
		config.NGROK_TRAFFIC_POLICY_CONFIG_KEY,
	)
}

func getHelmSecretTxt() string {
	return fmt.Sprintf(`
{{- if .Values.ngrok.enabled -}}
apiVersion: v1
kind: Secret
metadata:
  name: %s
data:
  NGROK_AUTHTOKEN: %s
{{- end -}}
`,
		config.NGROK_SECRET_NAME,
		base64.StdEncoding.EncodeToString([]byte(config.NGROK_AUTHTOKEN)),
	)
}

func getHelmDeploymentTxt(svcName string) string {
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
          image: %[2]s
          imagePullPolicy: IfNotPresent
          command:
            - ngrok
            - http
            - %[1]s:{{ .Values.ngrok.port }}
            - --traffic-policy-file
            - %[5]s/%[6]s
          envFrom:
            - secretRef:
                name: %[3]s
          volumeMounts:
            - name: %[4]s
              mountPath: %[5]s
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
		config.NGROK_IMAGE,
		config.NGROK_SECRET_NAME,
		config.NGROK_CONFIG_MAP_NAME,
		config.NGROK_CONFIG_VOL_MOUNT_PATH,
		config.NGROK_TRAFFIC_POLICY_CONFIG_KEY,
	)
}

func getHelmServiceTxt(svcName string) string {
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

func getHelmServiceAccountTxt(svcName string) string {
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

func installChart(
	actionConfig *helmaction.Configuration,
	mainExt *config.XMiniEnv,
	svcExt *config.XMiniEnvK8sService,
	svc *types.ServiceConfig,
	dryRun bool,
) error {
	values, err := svcExt.ToValuesMap()
	if err != nil {
		return err
	}

	defaultTimeout := "2m"
	timeout := svcExt.DeploymentTimeout
	if timeout == nil {
		timeout = &defaultTimeout
	}

	parsedTimeout, err := time.ParseDuration(*timeout)
	if err != nil {
		return fmt.Errorf("invalid deploymentTimeout configuration: %s", err)
	}

	client := helmaction.NewInstall(actionConfig)
	client.ReleaseName = svc.Name
	client.Namespace = mainExt.K8s.Namespace
	client.CreateNamespace = false
	client.Wait = true
	client.Atomic = true
	client.Wait = true
	client.DryRun = dryRun
	client.Timeout = parsedTimeout

	chart, err := loadChart(svc.Name)
	if err != nil {
		return fmt.Errorf("failed to load in-memory chart: %s", err)
	}

	if _, err := client.Run(chart, values); err != nil {
		return fmt.Errorf("failed to install service chart %s: %s", svc.Name, err)
	}

	log.
		Info().
		Str("context", mainExt.K8s.Context).
		Str("namespace", mainExt.K8s.Namespace).
		Str("service", svc.Name).
		Msg("successfully installed service chart")

	return nil
}

func upgradeChart(
	actionConfig *helmaction.Configuration,
	mainExt *config.XMiniEnv,
	svcExt *config.XMiniEnvK8sService,
	svc *types.ServiceConfig,
	dryRun bool,
) error {
	values, err := svcExt.ToValuesMap()
	if err != nil {
		return err
	}

	defaultTimeout := "2m"
	timeout := svcExt.DeploymentTimeout
	if timeout == nil {
		timeout = &defaultTimeout
	}

	parsedTimeout, err := time.ParseDuration(*timeout)
	if err != nil {
		return fmt.Errorf("invalid deploymentTimeout configuration: %s", err)
	}

	client := helmaction.NewUpgrade(actionConfig)
	client.Namespace = mainExt.K8s.Namespace
	client.Atomic = true
	client.Wait = true
	client.CleanupOnFail = true
	client.DryRun = dryRun
	client.Timeout = parsedTimeout

	chart, err := loadChart(svc.Name)
	if err != nil {
		return fmt.Errorf("failed to load in-memory chart: %s", err)
	}

	if _, err := client.Run(svc.Name, chart, values); err != nil {
		return fmt.Errorf("failed to upgrade service chart %s: %s", svc.Name, err)
	}

	log.
		Info().
		Str("context", mainExt.K8s.Context).
		Str("namespace", mainExt.K8s.Namespace).
		Str("service", svc.Name).
		Msg("successfully upgraded service chart")

	return nil
}

func uninstallChart(
	actionConfig *helmaction.Configuration,
	mainExt *config.XMiniEnv,
	svc *types.ServiceConfig,
	dryRun bool,
) error {
	client := helmaction.NewUninstall(actionConfig)
	client.Wait = true
	client.IgnoreNotFound = true
	client.DryRun = dryRun

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
		Str("context", mainExt.K8s.Context).
		Str("service", response.Release.Name).
		Str("namespace", response.Release.Namespace).
		Msg("successfully uninstalled service")

	return nil
}

func getHelmActionConfig(
	extConfig *config.XMiniEnv,
) (*helmaction.Configuration, error) {
	settings := helmcli.New()
	settings.SetNamespace(extConfig.K8s.Namespace)
	settings.KubeContext = extConfig.K8s.Context

	actionConfig := new(helmaction.Configuration)

	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		settings.Namespace(),
		os.Getenv("HELM_DRIVER"),
		log.Printf,
	); err != nil {
		return nil, err
	}

	return actionConfig, nil
}

func createNamespaceIfNotExists(
	actionConfig *helmaction.Configuration,
	namespace string,
) error {
	clientset, err := actionConfig.KubernetesClientSet()
	if err != nil {
		return err
	}

	create := false

	_, err = clientset.
		CoreV1().
		Namespaces().
		Get(context.TODO(), namespace, k8smetav1.GetOptions{})

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
			&k8sv1.Namespace{Name: namespace},
			k8smetav1.CreateOptions{},
		)

	return err
}

func serviceReleaseExists(
	actionConfig *helmaction.Configuration,
	name string,
) bool {
	client := helmaction.NewGet(actionConfig)
	release, _ := client.Run(name)
	return release != nil
}
