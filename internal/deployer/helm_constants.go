package deployer

import (
	"encoding/base64"
	"fmt"
)

func helmChartYamlTmpl(svcName string) string {
	return fmt.Sprintf(`
type: application
name: %s
version: 0.1.0
appVersion: 0.1.0
description: A chart for deploying service service
`, svcName)
}

func helmNgrokSecretTmpl(authToken string) string {
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

const HELM_HELPERS_TMPL = `
{{/*
Expand the name of the chart.
*/}}
{{- define "generated.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "generated.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "generated.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "generated.labels" -}}
helm.sh/chart: {{ include "generated.chart" . }}
{{ include "generated.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "generated.selectorLabels" -}}
app.kubernetes.io/name: {{ include "generated.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "generated.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "generated.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
`

const HELM_DEPLOYMENT_VALUES_TMPL = `
replicas: 1

image:
  repository: ""
  pullPolicy: IfNotPresent
  tag: ""

imagePullSecrets: []

nameOverride: ""
fullnameOverride: ""

command: []

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

const HELM_NGROK_CONFIG_MAP_TMPL = `
{{- if .Values.ngrok.enabled -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Values.ngrok.configMapName }}
data:
  {{ .Values.ngrok.configKey }}: |
    version: 3
    endpoints:
      - name: {{ include "generated.fullname" . }}
        url: {{ .Values.ngrok.url }}
        description: "endpoint for service"
        upstream:
          url: {{ include "generated.fullname" . }}:{{ .Values.ngrok.port }}
        traffic_policy: {{ .Values.ngrok.trafficPolicy }}
{{- end -}}
`

const HELM_DEPLOYMENT_TMPL = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "generated.fullname" . }}
  labels:
    {{- include "generated.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicas }}
  selector:
    matchLabels:
      {{- include "generated.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      {{- with .Values.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "generated.labels" . | nindent 8 }}
        {{- with .Values.podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "generated.serviceAccountName" . }}
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
          {{- if .Values.command }}
          {{- with .Values.command }}
          command:
            {{ . | toYaml | nindent 12 }}
          {{- end }}
          {{- end }}
          {{- if .Values.service.ports }}
          ports:
            {{- range .Values.service.ports }}
            - name: {{ .containerPortName }}
              containerPort: {{ .containerPort }}
              protocol: {{ .protocol }}
            {{- end }}
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
            - {{ include "generated.fullname" . }}
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
`

const HELM_SERVICE_TMPL = `
{{- if .Values.service.create -}}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "generated.fullname" . }}
  labels:
    {{- include "generated.labels" . | nindent 4 }}
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
    {{- include "generated.selectorLabels" . | nindent 4 }}
{{- end -}}
`

const HELM_SERVICE_ACCOUNT_TMPL = `
{{- if .Values.serviceAccount.create -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "generated.serviceAccountName" . }}
  labels:
    {{- include "generated.labels" . | nindent 4 }}
  {{- with .Values.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
automountServiceAccountToken: {{ .Values.serviceAccount.automount }}
{{- end -}}
`

const HELM_JOB_VALUES_TMPL = `
image:
  repository: ""
  pullPolicy: IfNotPresent
  tag: ""

imagePullSecrets: []

nameOverride: ""
fullnameOverride: ""

serviceAccount:
  create: true
  automount: true
  annotations: {}
  name: ""

service:
  ports: []

env: {}

podAnnotations: {}
podLabels: {}
podSecurityContext: {}
securityContext: {}

resources: {}

nodeSelector: {}
tolerations: []
affinity: {}
`

const HELM_JOB_TMPL = `
apiVersion: batch/v1
kind: Job
metadata:
  name: "{{ include "generated.fullname" . }}"
spec:
  backoffLimit: 1
  template:
    metadata:
      {{- with .Values.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "generated.labels" . | nindent 8 }}
        {{- with .Values.podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "generated.serviceAccountName" . }}
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
          {{- if .Values.command }}
          {{- with .Values.command }}
          command:
            {{ . | toYaml | nindent 12 }}
          {{- end }}
          {{- end }}
          {{- if .Values.service.ports }}
          ports:
            {{- range .Values.service.ports }}
            - name: {{ .containerPortName }}
              containerPort: {{ .containerPort }}
              protocol: {{ .protocol }}
            {{- end }}
          {{- end }}
          {{- if .Values.env }}
          env:
            {{- range $k, $v := .Values.env }}
            - name: {{ $k }}
              value: {{ $v }}
            {{- end }}
          {{- end }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 12 }}
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
      restartPolicy: Never
`
