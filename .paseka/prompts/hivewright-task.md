{{if .Task}}
## Task
{{.Task}}
{{end}}
{{if and .Interactive (eq .Adapter "cursor")}}
{{template "cursor-interactive-kickoff" .}}
{{end}}
