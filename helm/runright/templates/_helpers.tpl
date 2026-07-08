{{/*
Expand the name of the chart.
*/}}
{{- define "runright.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "runright.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "runright.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "runright.labels" -}}
helm.sh/chart: {{ include "runright.chart" . }}
{{ include "runright.selectorLabels" . }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "runright.selectorLabels" -}}
app.kubernetes.io/name: {{ include "runright.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "runright.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "runright.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Resolve the PostgreSQL DSN. Uses the bundled chart DSN if enabled,
or the externalDSN value otherwise.
*/}}
{{- define "runright.dsn" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "postgres://%s:%s@%s-postgresql:5432/%s?sslmode=disable"
    .Values.postgresql.auth.username
    .Values.postgresql.auth.password
    (include "runright.fullname" .)
    .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.externalDSN }}
{{- end }}
{{- end }}
