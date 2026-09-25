// {{ .Name }} is a generated database row.
type {{ .Name }} struct {
{{- range .Fields }}
    {{ .Name }} {{ if eq .Col.DataType "jsonb" }}[]byte{{ else }}{{ retype .Type }}{{ end }}
{{- end }}
}
const Table_{{ .Name }} = "{{ .Table.TableName }}"
const (
{{- $name := .Name }}
{{- range .Fields }}
    {{ $name }}_{{ .Name }} = "{{ .Col.ColumnName }}"
{{- end }}
)
func (r {{ .Name }}) Columns() string { return "{{ colnames .Fields }}" }
func (r {{ .Name }}) Values() []any { return []any{ {{ range .Fields }}r.{{ .Name }},{{ end }} } }
