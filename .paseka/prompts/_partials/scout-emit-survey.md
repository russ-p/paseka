{{template "insight-intro" .}}

| `payload.kind` | Role | Prompt memory |
| -------------- | ---- | ------------- |
| `context.note` | Finding or scope fact | yes |
| `review.note` | Finding with severity (non-gate) | yes |
| `run.summary` | Short ranked digest | yes |
| `task.plan` | Optional builder slices when findings are queue-ready | no |

{{template "insight-kind-context-note" .}}
{{template "insight-kind-review-note" .}}
{{template "insight-kind-run-summary" .}}
{{template "insight-kind-task-plan" .}}
