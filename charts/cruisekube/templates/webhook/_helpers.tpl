{{/*
Expand the name of the chart.
*/}}
{{- define "cruisekubeWebhook.name" -}}
{{- default "webhook" .Values.cruisekubeWebhook.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "cruisekubeWebhook.fullname" -}}
{{- if .Values.cruisekubeWebhook.fullnameOverride }}
{{- .Values.cruisekubeWebhook.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default "webhook" .Values.cruisekubeWebhook.nameOverride }}
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
{{- define "cruisekubeWebhook.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cruisekubeWebhook.labels" -}}
helm.sh/chart: {{ include "cruisekubeWebhook.chart" . }}
{{ include "cruisekubeWebhook.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cruisekubeWebhook.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cruisekubeWebhook.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "cruisekubeWebhook.serviceAccountName" -}}
{{- if .Values.cruisekubeWebhook.serviceAccount.create }}
{{- default (include "cruisekubeWebhook.fullname" .) .Values.cruisekubeWebhook.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.cruisekubeWebhook.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "cruisekubeWebhook.certGenServiceAccountName" -}}
{{- if .Values.cruisekubeWebhook.certGen.serviceAccount.create }}
{{- default (printf "%s-certgen" (include "cruisekubeWebhook.fullname" .)) .Values.cruisekubeWebhook.certGen.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.cruisekubeWebhook.certGen.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the TLS secret
*/}}
{{- define "cruisekubeWebhook.tlsSecretName" -}}
{{- printf "%s-tls" (include "cruisekubeWebhook.fullname" .) }}
{{- end }}


{{/*
Create the webhook service name
*/}}
{{- define "cruisekubeWebhook.webhookServiceName" -}}
{{- printf "%s-webhook" (include "cruisekubeWebhook.fullname" .) }}
{{- end }}

{{/*
Generate namespace selector for webhook
*/}}
{{- define "cruisekubeWebhook.mutatingWebhookConfigurationNamespaceSelector" -}}
matchExpressions:
- key: name
  operator: NotIn
  values:
  {{- range .Values.cruisekubeWebhook.mutatingWebhookConfiguration.namespaceSelector.excludeNamespaces }}
  - {{ . | quote }}
  {{- end }}
{{- end }}


{{/*
Get image tag
*/}}
{{- define "cruisekubeWebhook.imageTag" -}}
{{- .Values.cruisekubeWebhook.image.tag | default .Chart.AppVersion }}
{{- end }}
{{/*
ServiceMonitor labels - merges common labels with servicemonitor-specific labels
*/}}
{{- define "cruisekubeWebhook.serviceMonitorLabels" -}}
{{- $prometheusLabel := dict "release" "prometheus" }}
{{- $commonLabels := include "cruisekubeWebhook.labels" . | fromYaml }}
{{- $serviceMonitorLabels := mergeOverwrite $commonLabels $prometheusLabel .Values.cruisekubeWebhook.serviceMonitor.additionalLabels }}
{{- toYaml $serviceMonitorLabels }}
{{- end }}

