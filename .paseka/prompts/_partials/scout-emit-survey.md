## Publish events (survey only)

A survey reports; it does not start work. No `task.ready`, and no `task.plan`
unless the findings are ready to queue as builder slices.

| `payload.kind` | When |
| --- | --- |
| `run.summary` | Short ranked digest of the findings |
| `context.note` | One finding or scope fact the next bee needs |
| `review.note` | A finding carrying a `severity` |
| `task.plan` | Optional builder slices, queue-ready findings only |

Payloads and severities: skill `paseka-event-emit` → `references/insight.md`.
