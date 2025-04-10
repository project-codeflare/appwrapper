{{ define "packages" -}}

{{- range $idx, $val := .packages -}}
  {{- if .IsMain -}}
---
permalink: /api/{{ .DisplayName }}/
title: {{ .Title }}
classes: wide
description: Generated API reference documentation for {{ .DisplayName }}.
---
{{ .GetComment -}}
  {{- end -}}
{{- end }}

## Resource Types

{{ range .packages -}}
{{- range .VisibleTypes -}}
- [{{ .DisplayName }}]({{ .Link }})
{{ end -}}
{{- end -}}

{{ range .packages -}}
  {{ if ne .GroupName "" -}}
    {{/* For package with a group name, list all type definitions in it. */}}
    {{- range .VisibleTypes }}
      {{- if or .Referenced .IsExported -}}
{{ template "type" . }}
      {{- end -}}
    {{ end }}
  {{ else }}
    {{/* For package w/o group name, list only types referenced. */}}
    {{- range .VisibleTypes -}}
      {{- if .Referenced -}}
{{ template "type" . }}
      {{- end -}}
    {{- end }}
  {{- end }}
{{- end }}
{{- end }}
