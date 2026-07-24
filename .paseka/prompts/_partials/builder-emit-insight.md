{{template "insight-intro" .}}

| `payload.kind` | Role | Prompt memory |
| -------------- | ---- | ------------- |
| `run.summary` | Short run outcome for the next bee | yes |
| `context.note` | Trace/task context fact | yes |
| `trace.summary` | Flight Trail description (last work task only) | no |

{{template "insight-kind-run-summary" .}}
{{template "insight-kind-context-note" .}}
{{template "insight-kind-trace-summary" .}}
