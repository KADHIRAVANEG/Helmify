{{- define "manifests.fullname" -}}
{{- .Release.Name -}}-{{ .Chart.Name }}
{{- end }}

{{- define "manifests.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
