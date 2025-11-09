{{- define "hpavalidator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "hpavalidator.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "hpavalidator.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "hpavalidator.labels" -}}
app.kubernetes.io/name: {{ include "hpavalidator.name" . }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "hpavalidator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hpavalidator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "hpavalidator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "hpavalidator.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end }}
