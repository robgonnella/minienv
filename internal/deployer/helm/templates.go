package helm

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
description: A chart for deploying service
`, svcName)
}

func helmNgrokSecretTmpl(authToken string) string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: {{ .Values.secretName }}
data:
  NGROK_AUTHTOKEN: %s
`,
		base64.StdEncoding.EncodeToString([]byte(authToken)),
	)
}

const helpersTmpl = `
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

const deploymentValuesTmpl = `
replicas: 1

image:
  repository: ""
  pullPolicy: IfNotPresent
  tag: ""

imagePullSecrets: []

nameOverride: ""
fullnameOverride: ""

command: []

env: []
envFrom: []
secretEnv: {}

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

const ngrokValuesTmpl = `
replicas: 1

image:
  repository: ""
  pullPolicy: IfNotPresent
  tag: ""

command: []

# Read by the shared deployment template even though this chart renders no
# Service of its own; without the key the template errors on a nil map.
service:
  ports: []

strategy: {}

serviceAccount:
  create: true
  automount: true
  annotations: {}
  name: ""

configMapName: ""
configKey: ""
configVolMountPath: ""
secretName: ""

endpoints: []

podAnnotations: {}

env: []
envFrom: []
secretEnv: {}
volumes: []
volumeMounts: []
`

const ngrokConfigMapTmpl = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Values.configMapName }}
data:
  {{ .Values.configKey }}: |
    version: 3
    endpoints:
    {{- range .Values.endpoints }}
      - name: {{ .endpointName }}
        description: "Endpoint for {{ .serviceName }} service in namespace {{ .namespace }}"
        {{- if .url }}
        url: {{ .url }}
        {{- end }}
        upstream:
          url: {{ .serviceName }}:{{ .port }}
        {{- if .trafficPolicy }}
        traffic_policy:
          {{- .trafficPolicy | trim | nindent 10 }}
        {{- end }}
    {{- end }}
`

const filesConfigMapTmpl = `
{{- $files := .Files.Glob "files/configmap/*" }}
{{- if $files }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "generated.fullname" . }}
  labels:
    {{- include "generated.labels" . | nindent 4 }}
data:
  {{- $files.AsConfig | nindent 2 }}
{{- end }}
`

// #nosec G101 -- chart template text; the values arrive at render time.
const secretEnvTmpl = `
{{- with .Values.secretEnv }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "generated.fullname" $ }}
  labels:
    {{- include "generated.labels" $ | nindent 4 }}
type: Opaque
stringData:
  {{- toYaml . | nindent 2 }}
{{- end }}
`

const deploymentTmpl = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "generated.fullname" . }}
  labels:
    {{- include "generated.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicas }}
  {{- with .Values.strategy }}
  strategy:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "generated.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      {{- $files := .Files.Glob "files/configmap/*" }}
      {{- if or .Values.podAnnotations $files .Values.secretEnv }}
      annotations:
        {{- with .Values.podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
        {{- if $files }}
        checksum/configmap: {{ $files.AsConfig | sha256sum }}
        {{- end }}
        {{- with .Values.secretEnv }}
        checksum/secret: {{ toYaml . | sha256sum }}
        {{- end }}
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
          {{- with .Values.env }}
          env:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- if or .Values.envFrom .Values.secretEnv }}
          envFrom:
            {{- if .Values.secretEnv }}
            - secretRef:
                name: {{ include "generated.fullname" . }}
            {{- end }}
            {{- with .Values.envFrom }}
            {{- toYaml . | nindent 12 }}
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
`

const serviceTmpl = `
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
    - name: {{ .containerPortName }}
      port: {{ .containerPort }}
      targetPort: {{ .containerPortName }}
      protocol: {{ .protocol }}
    {{- end }}
  selector:
    {{- include "generated.selectorLabels" . | nindent 4 }}
{{- end -}}
`

const serviceAccountTmpl = `
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

const jobValuesTmpl = `
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

env: []
envFrom: []
secretEnv: {}

podAnnotations: {}
podLabels: {}
podSecurityContext: {}
securityContext: {}

resources: {}

volumes: []
volumeMounts: []

nodeSelector: {}
tolerations: []
affinity: {}
`

const jobTmpl = `
apiVersion: batch/v1
kind: Job
metadata:
  name: "{{ include "generated.fullname" . | trunc 54 | trimSuffix "-" }}-{{ randAlphaNum 8 | lower }}"
spec:
  backoffLimit: 1
  template:
    metadata:
      {{- $files := .Files.Glob "files/configmap/*" }}
      {{- if or .Values.podAnnotations $files .Values.secretEnv }}
      annotations:
        {{- with .Values.podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
        {{- if $files }}
        checksum/configmap: {{ $files.AsConfig | sha256sum }}
        {{- end }}
        {{- with .Values.secretEnv }}
        checksum/secret: {{ toYaml . | sha256sum }}
        {{- end }}
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
          {{- with .Values.env }}
          env:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- if or .Values.envFrom .Values.secretEnv }}
          envFrom:
            {{- if .Values.secretEnv }}
            - secretRef:
                name: {{ include "generated.fullname" . }}
            {{- end }}
            {{- with .Values.envFrom }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
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
      restartPolicy: Never
`
