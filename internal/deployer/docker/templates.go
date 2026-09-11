package docker

const ngrokConfigTmpl = `
version: 3
endpoints:
{{- range . }}
  - name: {{ .EndpointName }}
    description: "Endpoint for {{ .ServiceName }} service in namespace {{ .Namespace }}"
    {{- if .URL }}
    url: {{ .URL }}
    {{- end }}
    upstream:
      url: {{ .ServiceName }}:{{ .Port }}
    {{- if .TrafficPolicy }}
    traffic_policy:
      {{- .TrafficPolicy | trim | nindent 10 }}
    {{- end }}
{{- end }}
`
