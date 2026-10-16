{{template "insight-intro" .}}

| `payload.kind` | Role | Prompt memory |
| -------------- | ---- | ------------- |
| `context.note` | One-line probe verdict for the tick | yes |

A probe tick publishes `context.note` **live** (never `--defer`): the completion contract
is checked against this run's `events.ndjson`, and a deferred event is not guaranteed to be
there yet. Keep the summary to a single `probe ok …` / `probe degraded …` line.

{{template "insight-kind-context-note" .}}