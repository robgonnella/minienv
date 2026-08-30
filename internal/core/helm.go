package core

import (
	"fmt"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

func loadChart(svcName string) (*chart.Chart, error) {
	files := []*loader.BufferedFile{
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
	}

	return loader.LoadFiles(files)
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
            - name: {{ .name }}
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
`, svcName)
}

func getHelmServiceTxt(svcName string) string {
	return fmt.Sprintf(`
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
    - name: {{ .name }}
      port: {{ .port }}
      targetPort: {{ .name }}
      protocol: {{ .protocol }}
    {{- end }}
  selector:
    {{- include "%[1]s.selectorLabels" . | nindent 4 }}
`, svcName)
}

func getHelmServiceAccountTxt(svcName string) string {
	return fmt.Sprintf(`
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
`, svcName)
}
