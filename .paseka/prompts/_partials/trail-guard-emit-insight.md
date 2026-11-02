## INSIGHT events

Use `type: INSIGHT` for context, audit, and dashboard narrative. INSIGHT events do **not** drive
workflow routing. Publish **only** the `payload.kind` values listed for this role below. Do not emit
`human.feedback` (Beekeeper / HITL only).

| `payload.kind` | Role | Prompt memory |
| -------------- | ---- | ------------- |
| `task.plan` | Cheap follow-up tasks, each depending on this closing round | no (operational) |
| `pr.body` | Authoritative pull-request description | no (operational) |
| `trace.summary` | Flight trail description and default merge commit body | no (operational) |
| `context.note` | Trail-level fact the next closing round should know | yes |
| `review.note` | Non-blocking observation about the change set | yes |

Prompt memory is **task-scoped**: a narrative note carrying `"taskId": "{{.TaskID}}"` is projected to
that task only, not to the next closing round. Leave `taskId` out of a note you want the next round
to read.

### `task.plan` — cheap follow-up tasks

One event with the full `tasks` array. Every task carries `dependsOn: ["{{.TaskID}}"]` so it runs
after this closing round and keeps the merge gate closed until it does. No `task.ready` kick: the
ledger promotes the oldest eligible planned task on every task completion, so the dependency chain
is the scheduler. Ignore the shared `task.ready` guidance above for this role.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[{"taskId":"fix-01","title":"Deduplicate the retry helper","body":"...","bee":"builder","dependsOn":["{{.TaskID}}"]}]}}
EOF
```

### `pr.body` — pull request description (**required** on the closing round)

You **must** emit one `INSIGHT/pr.body` in markdown describing the pull request. This is a
prompt-level obligation, not a runtime completion-contract failure. Max 8000 characters after trim.
Last write wins: the next closing round replaces it.

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"pr.body","body":"## Why\nShips the isolated trail as a reviewable pull request.\n\n## What changed\n- ..."}}
EOF
```

### `trace.summary` — flight trail summary (**required** on the closing round)

You **must** emit one `INSIGHT/trace.summary` with one to three sentences of plain prose describing
what this flight trail accomplished. Do not use conventional commit prefixes (`feat:`, `fix:`, ...).

Do not confuse the names:

- `paseka proposal approve --summary` → `VERIFICATION/task.completed` completion note
- `mergeMessage` / `--merge-message` → git merge commit **subject**
- `INSIGHT/trace.summary` → trail description and default merge commit **body**

```bash
paseka event emit --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"trace.summary","summary":"Closed the trail: audited every slice, filed two cheap follow-ups, and wrote the merge description."}}
EOF
```

### `context.note` and `review.note` — trace-scoped observations

Omit `taskId` so the note reaches the next closing round. Use `context.note` for a fact the next
round must know, `review.note` for a non-blocking observation about the change set with a
`severity` of `low`, `medium`, or `high`.

```bash
paseka event emit --defer --stdin <<'EOF'
{"traceId":"{{.TraceID}}","agentId":"{{.AgentID}}","type":"INSIGHT","payload":{"kind":"review.note","severity":"low","summary":"Two slices duplicate the retry helper; filed as fix-1."}}
EOF
```
