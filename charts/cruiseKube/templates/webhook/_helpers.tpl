{{/*
Expand the name of the chart.
*/}}
{{- define "cruiseKubeWebhook.name" -}}
{{- default "webhook" .Values.cruiseKubeWebhook.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "cruiseKubeWebhook.fullname" -}}
{{- if .Values.cruiseKubeWebhook.fullnameOverride }}
{{- .Values.cruiseKubeWebhook.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default "webhook" .Values.cruiseKubeWebhook.nameOverride }}
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
{{- define "cruiseKubeWebhook.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cruiseKubeWebhook.labels" -}}
helm.sh/chart: {{ include "cruiseKubeWebhook.chart" . }}
{{ include "cruiseKubeWebhook.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cruiseKubeWebhook.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cruiseKubeWebhook.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "cruiseKubeWebhook.serviceAccountName" -}}
{{- if .Values.cruiseKubeWebhook.serviceAccount.create }}
{{- default (include "cruiseKubeWebhook.fullname" .) .Values.cruiseKubeWebhook.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.cruiseKubeWebhook.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "cruiseKubeWebhook.certGenServiceAccountName" -}}
{{- if .Values.cruiseKubeWebhook.certGen.serviceAccount.create }}
{{- default (printf "%s-certgen" (include "cruiseKubeWebhook.fullname" .)) .Values.cruiseKubeWebhook.certGen.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.cruiseKubeWebhook.certGen.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the TLS secret
*/}}
{{- define "cruiseKubeWebhook.tlsSecretName" -}}
{{- printf "%s-tls" (include "cruiseKubeWebhook.fullname" .) }}
{{- end }}


{{/*
Create the webhook service name
*/}}
{{- define "cruiseKubeWebhook.webhookServiceName" -}}
{{- printf "%s-webhook" (include "cruiseKubeWebhook.fullname" .) }}
{{- end }}

{{/*
Generate namespace selector for webhook
*/}}
{{- define "cruiseKubeWebhook.mutatingWebhookConfigurationNamespaceSelector" -}}
matchExpressions:
- key: name
  operator: NotIn
  values:
  {{- range .Values.cruiseKubeWebhook.mutatingWebhookConfiguration.namespaceSelector.excludeNamespaces }}
  - {{ . | quote }}
  {{- end }}
{{- end }}


{{/*
Get image tag
*/}}
{{- define "cruiseKubeWebhook.imageTag" -}}
{{- .Values.cruiseKubeWebhook.image.tag | default .Chart.AppVersion }}
{{- end }}
{{/*
ServiceMonitor labels - merges common labels with servicemonitor-specific labels
*/}}
{{- define "cruiseKubeWebhook.serviceMonitorLabels" -}}
{{- $prometheusLabel := dict "release" "prometheus" }}
{{- $commonLabels := include "cruiseKubeWebhook.labels" . | fromYaml }}
{{- $serviceMonitorLabels := mergeOverwrite $commonLabels $prometheusLabel .Values.cruiseKubeWebhook.serviceMonitor.additionalLabels }}
{{- toYaml $serviceMonitorLabels }}
{{- end }}

