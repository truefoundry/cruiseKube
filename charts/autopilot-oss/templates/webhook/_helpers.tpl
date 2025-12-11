{{/*
Expand the name of the chart.
*/}}
{{- define "autopilotWebhook.name" -}}
{{- default .Chart.Name .Values.autopilotWebhook.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "autopilotWebhook.fullname" -}}
{{- if .Values.autopilotWebhook.fullnameOverride }}
{{- .Values.autopilotWebhook.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.autopilotWebhook.nameOverride }}
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
{{- define "autopilotWebhook.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "autopilotWebhook.labels" -}}
helm.sh/chart: {{ include "autopilotWebhook.chart" . }}
{{ include "autopilotWebhook.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "autopilotWebhook.selectorLabels" -}}
app.kubernetes.io/name: {{ include "autopilotWebhook.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "autopilotWebhook.serviceAccountName" -}}
{{- if .Values.autopilotWebhook.serviceAccount.create }}
{{- default (include "autopilotWebhook.fullname" .) .Values.autopilotWebhook.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.autopilotWebhook.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "autopilotWebhook.certGenServiceAccountName" -}}
{{- if .Values.autopilotWebhook.certGen.serviceAccount.create }}
{{- default (printf "%s-certgen" (include "autopilotWebhook.fullname" .)) .Values.autopilotWebhook.certGen.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.autopilotWebhook.certGen.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the TLS secret
*/}}
{{- define "autopilotWebhook.tlsSecretName" -}}
{{- printf "%s-tls" (include "autopilotWebhook.fullname" .) }}
{{- end }}


{{/*
Create the webhook service name
*/}}
{{- define "autopilotWebhook.webhookServiceName" -}}
{{- printf "%s-webhook" (include "autopilotWebhook.fullname" .) }}
{{- end }}

{{/*
Generate namespace selector for webhook
*/}}
{{- define "autopilotWebhook.mutatingWebhookConfigurationNamespaceSelector" -}}
matchExpressions:
- key: name
  operator: NotIn
  values:
  {{- range .Values.autopilotWebhook.mutatingWebhookConfiguration.namespaceSelector.excludeNamespaces }}
  - {{ . | quote }}
  {{- end }}
{{- end }}


{{/*
Get image tag
*/}}
{{- define "autopilotWebhook.imageTag" -}}
{{- .Values.autopilotWebhook.image.tag | default .Chart.AppVersion }}
{{- end }}
{{/*
ServiceMonitor labels - merges common labels with servicemonitor-specific labels
*/}}
{{- define "autopilotWebhook.serviceMonitorLabels" -}}
{{- $prometheusLabel := dict "release" "prometheus" }}
{{- $commonLabels := include "autopilotWebhook.labels" . | fromYaml }}
{{- $serviceMonitorLabels := mergeOverwrite $commonLabels $prometheusLabel .Values.autopilotWebhook.serviceMonitor.additionalLabels }}
{{- toYaml $serviceMonitorLabels }}
{{- end }}

