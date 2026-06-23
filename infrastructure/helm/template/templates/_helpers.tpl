{{- define "realtime-parking-bz-shim.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "realtime-parking-bz-shim.fullname" -}}
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

{{- define "realtime-parking-bz-shim.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "realtime-parking-bz-shim.labels" -}}
helm.sh/chart: {{ include "realtime-parking-bz-shim.chart" . }}
{{ include "realtime-parking-bz-shim.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "realtime-parking-bz-shim.selectorLabels" -}}
app.kubernetes.io/name: {{ include "realtime-parking-bz-shim.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
