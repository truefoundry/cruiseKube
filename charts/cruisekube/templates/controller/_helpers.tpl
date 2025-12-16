{{/*
Expand the name of the chart.
*/}}
{{- define "cruisekubeController.name" -}}
{{- default "controller" .Values.cruisekubeController.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "cruisekubeController.fullname" -}}
{{- if .Values.cruisekubeController.fullnameOverride }}
{{- .Values.cruisekubeController.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default "controller" .Values.cruisekubeController.nameOverride }}
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
{{- define "cruisekubeController.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cruisekubeController.labels" -}}
helm.sh/chart: {{ include "cruisekubeController.chart" . }}
{{ include "cruisekubeController.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cruisekubeController.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cruisekubeController.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "cruisekubeController.serviceAccountName" -}}
{{- if .Values.cruisekubeController.serviceAccount.create }}
{{- default (include "cruisekubeController.fullname" .) .Values.cruisekubeController.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.cruisekubeController.serviceAccount.name }}
{{ end }}
{{- end }}

{{/*
Get image tag
*/}}
{{- define "cruisekubeController.imageTag" -}}
{{- .Values.cruisekubeController.image.tag | default .Chart.AppVersion }}
{{- end }}
{{/*
ServiceMonitor labels - merges common labels with servicemonitor-specific labels
*/}}
{{- define "cruisekubeController.serviceMonitorLabels" -}}
{{- $prometheusLabel := dict "release" "prometheus" }}
{{- $commonLabels := include "cruisekubeController.labels" . | fromYaml }}
{{- $serviceMonitorLabels := mergeOverwrite $commonLabels $prometheusLabel .Values.cruisekubeController.serviceMonitor.additionalLabels }}
{{- toYaml $serviceMonitorLabels }}
{{- end }}
