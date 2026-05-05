{{/*
Expand the name of the chart.
*/}}
{{- define "kagent-tools.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kagent-tools.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- if not .Values.nameOverride }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kagent-tools.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kagent-tools.labels" -}}
helm.sh/chart: {{ include "kagent-tools.chart" . }}
{{ include "kagent-tools.selectorLabels" . }}
{{- if .Chart.Version }}
app.kubernetes.io/version: {{ .Chart.Version | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kagent-tools.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kagent-tools.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*Default provider name*/}}
{{- define "kagent-tools.defaultProviderName" -}}
{{ .Values.providers.default | default "openAI" | lower}}
{{- end }}

{{/*Default model name*/}}
{{- define "kagent-tools.defaultModelConfigName" -}}
default-model-config
{{- end }}

{{/*
Expand the namespace of the release.
Allows overriding it for multi-namespace deployments in combined charts.
*/}}
{{- define "kagent-tools.namespace" -}}
{{- default .Release.Namespace .Values.namespaceOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Service account name: default when useDefaultServiceAccount is true, otherwise the chart fullname.
*/}}
{{- define "kagent-tools.serviceAccountName" -}}
{{- if .Values.useDefaultServiceAccount }}default{{- else }}{{ include "kagent-tools.fullname" . }}{{- end }}
{{- end }}

{{/*
Watch namespaces - comma-separated list for controllers that watch a subset of namespaces.
Precedence: controller.watchNamespaces (explicit override) > rbac.namespaces > empty (watch all).
*/}}
{{- define "kagent-tools.watchNamespaces" -}}
{{- $ctrl := index .Values "controller" | default dict -}}
{{- if index $ctrl "watchNamespaces" -}}
{{- index $ctrl "watchNamespaces" | uniq | join "," -}}
{{- else if and .Values.rbac .Values.rbac.namespaces -}}
{{- .Values.rbac.namespaces | uniq | join "," -}}
{{- end -}}
{{- end }}

{{/*
Guards on the rbac block
*/}}
{{- define "kagent-tools.rbac.validate" -}}
{{- if and .Values.rbac (hasKey .Values.rbac "clusterScoped") -}}
{{- fail "rbac.clusterScoped has been removed. Leave rbac.namespaces empty for cluster-scoped RBAC, or set rbac.namespaces=[<ns>, ...] for namespaced RBAC." -}}
{{- end -}}
{{- if and .Values.rbac .Values.rbac.namespaces -}}
{{- $installNs := include "kagent-tools.namespace" . -}}
{{- if not (has $installNs .Values.rbac.namespaces) -}}
{{- fail (printf "rbac.namespaces is set but does not include the install namespace %q" $installNs) -}}
{{- end -}}
{{- end -}}
{{- end -}}
