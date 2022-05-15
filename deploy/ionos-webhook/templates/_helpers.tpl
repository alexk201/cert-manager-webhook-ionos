{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "webhook-ionos.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "webhook-ionos.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "webhook-ionos.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "webhook-ionos.selfSignedIssuer" -}}
{{ printf "%s-selfsign" (include "webhook-ionos.fullname" .) }}
{{- end -}}

{{- define "webhook-ionos.rootCAIssuer" -}}
{{ printf "%s-ca" (include "webhook-ionos.fullname" .) }}
{{- end -}}

{{- define "webhook-ionos.rootCACertificate" -}}
{{ printf "%s-ca" (include "webhook-ionos.fullname" .) }}
{{- end -}}

{{- define "webhook-ionos.servingCertificate" -}}
{{ printf "%s-webhook-tls" (include "webhook-ionos.fullname" .) }}
{{- end -}}
