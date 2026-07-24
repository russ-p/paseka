{{template "insight-intro" .}}

| `payload.kind` | Role | Prompt memory |
| -------------- | ---- | ------------- |
| `review.note` | Reviewer observation (non-gate) | yes |
| `context.note` | Trace/task context fact | yes |
| `trace.summary` | Flight Trail description (last work task only) | no |

{{template "insight-kind-review-note" .}}
{{template "insight-kind-context-note" .}}
{{template "insight-kind-trace-summary" .}}
