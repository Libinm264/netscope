{{/*
Expand the name of the chart.
*/}}
{{- define "netscope.name" -}}
{{- default .Chart.Name .Values.global.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "netscope.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: netscope
{{- end }}

{{/*
Hub API selector labels
*/}}
{{- define "netscope.hub.selectorLabels" -}}
app.kubernetes.io/name: netscope-hub
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Hub Web selector labels
*/}}
{{- define "netscope.web.selectorLabels" -}}
app.kubernetes.io/name: netscope-hub-web
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Agent selector labels
*/}}
{{- define "netscope.agent.selectorLabels" -}}
app.kubernetes.io/name: netscope-agent
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
