{{/*
Expand the name of the chart.
*/}}
{{- define "cruiseKubeController.name" -}}
{{- default "controller" .Values.cruiseKubeController.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "cruiseKubeController.fullname" -}}
{{- if .Values.cruiseKubeController.fullnameOverride }}
{{- .Values.cruiseKubeController.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default "controller" .Values.cruiseKubeController.nameOverride }}
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
{{- define "cruiseKubeController.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cruiseKubeController.labels" -}}
helm.sh/chart: {{ include "cruiseKubeController.chart" . }}
{{ include "cruiseKubeController.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cruiseKubeController.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cruiseKubeController.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "cruiseKubeController.serviceAccountName" -}}
{{- if .Values.cruiseKubeController.serviceAccount.create }}
{{- default (include "cruiseKubeController.fullname" .) .Values.cruiseKubeController.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.cruiseKubeController.serviceAccount.name }}
{{ end }}
{{- end }}

{{/*
Get image tag
*/}}
{{- define "cruiseKubeController.imageTag" -}}
{{- .Values.cruiseKubeController.image.tag | default .Chart.AppVersion }}
{{- end }}
{{/*
ServiceMonitor labels - merges common labels with servicemonitor-specific labels
*/}}
{{- define "cruiseKubeController.serviceMonitorLabels" -}}
{{- $prometheusLabel := dict "release" "prometheus" }}
{{- $commonLabels := include "cruiseKubeController.labels" . | fromYaml }}
{{- $serviceMonitorLabels := mergeOverwrite $commonLabels $prometheusLabel .Values.cruiseKubeController.serviceMonitor.additionalLabels }}
{{- toYaml $serviceMonitorLabels }}
{{- end }}
