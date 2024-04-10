{{ define "packages" -}}

{{- range $idx, $val := .packages -}}
  {{- if .IsMain -}}
---
permalink: /api{{ .DisplayName }}/
title: {{ .Title }}
classes: wide
---
{{ .GetComment -}}
  {{- end -}}
{{- end }}

Generated API reference documentation for {{ .DisplayName }}.

## Resource Types

{{ range .packages -}}
  {{- range .VisibleTypes -}}
- [{{ .DisplayName }}]({{ .Link }})
  {{ end }}
{{- end -}}

{{ range .packages }}
  {{ if ne .GroupName "" -}}
    {{/* For package with a group name, list all type definitions in it. */}}
    {{- range .VisibleTypes }}
      {{- if or .Referenced .IsExported -}}
{{ template "type" . }}
      {{- end -}}
    {{ end }}
  {{ else }}
    {{/* For package w/o group name, list only types referenced. */}}
    {{ $isConfig := (eq .GroupName "") }}
    {{- range .VisibleTypes -}}
      {{- if or .Referenced $isConfig -}}
{{ template "type" . }}
      {{- end -}}
    {{- end }}
  {{- end }}
{{- end }}
{{- end }}
