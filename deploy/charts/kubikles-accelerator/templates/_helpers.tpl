{{- define "kubikles-accelerator.fullname" -}}
{{- printf "%s-kubikles-accelerator" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "kubikles-accelerator.clusterName" -}}
{{- $base := printf "%s-%s-accelerator" .Release.Namespace .Release.Name | lower | replace "_" "-" | trunc 54 | trimSuffix "-" -}}
{{- printf "%s-%s" $base (sha256sum (printf "%s/%s" .Release.Namespace .Release.Name) | trunc 8) -}}
{{- end }}
{{- define "kubikles-accelerator.labels" -}}
app.kubernetes.io/name: kubikles-accelerator
app.kubernetes.io/instance: {{ .Release.Name | quote }}
app.kubernetes.io/component: accelerator
app.kubernetes.io/part-of: kubikles
app.kubernetes.io/managed-by: {{ .Release.Service | quote }}
{{- end }}
