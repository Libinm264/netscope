{{/*
Expand the name of the chart.
*/}}
{{- define "nexor.name" -}}
{{- default .Chart.Name .Values.global.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "nexor.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: nexor
{{- end }}

{{/*
Hub API selector labels
*/}}
{{- define "nexor.hub.selectorLabels" -}}
app.kubernetes.io/name: nexor-hub
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Hub Web selector labels
*/}}
{{- define "nexor.web.selectorLabels" -}}
app.kubernetes.io/name: nexor-hub-web
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Agent selector labels
*/}}
{{- define "nexor.agent.selectorLabels" -}}
app.kubernetes.io/name: nexor-agent
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
