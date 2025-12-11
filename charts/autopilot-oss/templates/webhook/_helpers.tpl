{{- if .Values.autopilot-webhook.enabled }}
{{/*
Expand the name of the chart.
*/}}
{{- define "autopilot-webhook.name" -}}
{{- default .Chart.Name .Values.autopilot-webhook.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "autopilot-webhook.fullname" -}}
{{- if .Values.autopilot-webhook.fullnameOverride }}
{{- .Values.autopilot-webhook.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.autopilot-webhook.nameOverride }}
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
{{- define "autopilot-webhook.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "autopilot-webhook.labels" -}}
helm.sh/chart: {{ include "autopilot-webhook.chart" . }}
{{ include "autopilot-webhook.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "autopilot-webhook.selectorLabels" -}}
app.kubernetes.io/name: {{ include "autopilot-webhook.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "autopilot-webhook.serviceAccountName" -}}
{{- if .Values.autopilot-webhook.serviceAccount.create }}
{{- default (include "autopilot-webhook.fullname" .) .Values.autopilot-webhook.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.autopilot-webhook.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "autopilot-webhook.certGenServiceAccountName" -}}
{{- if .Values.autopilot-webhook.certGen.serviceAccount.create }}
{{- default (printf "%s-certgen" (include "autopilot-webhook.fullname" .)) .Values.autopilot-webhook.certGen.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.autopilot-webhook.certGen.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the TLS secret
*/}}
{{- define "autopilot-webhook.tlsSecretName" -}}
{{- printf "%s-tls" (include "autopilot-webhook.fullname" .) }}
{{- end }}


{{/*
Create the webhook service name
*/}}
{{- define "autopilot-webhook.webhookServiceName" -}}
{{- printf "%s-webhook" (include "autopilot-webhook.fullname" .) }}
{{- end }}

{{/*
Generate namespace selector for webhook
*/}}
{{- define "autopilot-webhook.mutatingWebhookConfigurationNamespaceSelector" -}}
matchExpressions:
- key: name
  operator: NotIn
  values:
  {{- range .Values.autopilot-webhook.mutatingWebhookConfiguration.namespaceSelector.excludeNamespaces }}
  - {{ . | quote }}
  {{- end }}
{{- end }}


{{/*
Get image tag
*/}}
{{- define "autopilot-webhook.imageTag" -}}
{{- .Values.autopilot-webhook.image.tag | default .Chart.AppVersion }}
{{- end }}
{{/*
ServiceMonitor labels - merges common labels with servicemonitor-specific labels
*/}}
{{- define "autopilot-webhook.serviceMonitorLabels" -}}
{{- $prometheusLabel := dict "release" "prometheus" }}
{{- $commonLabels := include "autopilot-webhook.labels" . | fromYaml }}
{{- $serviceMonitorLabels := mergeOverwrite $commonLabels $prometheusLabel .Values.autopilot-webhook.serviceMonitor.additionalLabels }}
{{- toYaml $serviceMonitorLabels }}
{{- end }}
{{- end }}
