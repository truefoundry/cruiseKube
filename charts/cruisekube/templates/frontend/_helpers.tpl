{{/*
Expand the name of the chart.
*/}}
{{- define "cruisekubeFrontend.name" -}}
{{- default "frontend" .Values.cruisekubeFrontend.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "cruisekubeFrontend.fullname" -}}
{{- if .Values.cruisekubeFrontend.fullnameOverride }}
{{- .Values.cruisekubeFrontend.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default "frontend" .Values.cruisekubeFrontend.nameOverride }}
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
{{- define "cruisekubeFrontend.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cruisekubeFrontend.labels" -}}
helm.sh/chart: {{ include "cruisekubeFrontend.chart" . }}
{{ include "cruisekubeFrontend.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cruisekubeFrontend.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cruisekubeFrontend.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Get image tag
*/}}
{{- define "cruisekubeFrontend.imageTag" -}}
{{- .Values.cruisekubeFrontend.image.tag | default .Chart.AppVersion }}
{{- end }}

