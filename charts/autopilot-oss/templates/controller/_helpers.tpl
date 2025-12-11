{{/*
Expand the name of the chart.
*/}}
{{- define "autopilotController.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "autopilotController.fullname" -}}
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
{{- define "autopilotController.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "autopilotController.labels" -}}
helm.sh/chart: {{ include "autopilotController.chart" . }}
{{ include "autopilotController.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "autopilotController.selectorLabels" -}}
app.kubernetes.io/name: {{ include "autopilotController.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "autopilotController.serviceAccountName" -}}
{{- if .Values.autopilotController.serviceAccount.create }}
{{- default (include "autopilotController.fullname" .) .Values.autopilotController.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.autopilotController.serviceAccount.name }}
{{ end }}
{{- end }}

{{/*
Get image tag
*/}}
{{- define "autopilotController.imageTag" -}}
{{- .Values.autopilotController.image.tag | default .Chart.AppVersion }}
{{- end }}
{{/*
ServiceMonitor labels - merges common labels with servicemonitor-specific labels
*/}}
{{- define "autopilotController.serviceMonitorLabels" -}}
{{- $prometheusLabel := dict "release" "prometheus" }}
{{- $commonLabels := include "autopilotController.labels" . | fromYaml }}
{{- $serviceMonitorLabels := mergeOverwrite $commonLabels $prometheusLabel .Values.autopilotController.serviceMonitor.additionalLabels }}
{{- toYaml $serviceMonitorLabels }}
{{- end }}
