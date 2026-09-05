# Event contracts

Paseka choreography uses four domain event types on NATS / JetStream:
`SIGNAL`, `INSIGHT`, `MUTATION`, and `VERIFICATION`. Every event carries a
`traceId`, producer `agentId`, and a JSON payload with `kind`.

```json
{
  "protocolVersion": "agent-runtime.v1",
  "traceId": "trace-auth-01",
  "agentId": "builder-01",
  "type": "SIGNAL",
  "payload": {
    "kind": "task.ready",
    "taskId": "task-1"
  }
}
```

Use `paseka event validate --stdin` before publishing custom events and
`paseka event emit --stdin` to validate and publish them. The subject includes
the colony namespace, event type, and payload kind.

## Event-type roles

| Type | Meaning | Typical effect |
| ---- | ------- | -------------- |
| `SIGNAL` | Trigger or platform control | dispatch, lifecycle update, operator action |
| `INSIGHT` | Plan, metadata, or narrative memory | ledger projection or prompt context |
| `MUTATION` | Proposed code change | reviewer dispatch and proposal gate |
| `VERIFICATION` | Gate result or completed work | retry, approval, or task completion |

`LOG`, `PROGRESS`, `TOOL_CALL`, and `ASSISTANT_TEXT` are run-lifecycle records,
not domain events published to the choreography bus.

## Platform-validated kinds

These kinds have stable type and payload validation in `internal/protocol`.

| Type | `payload.kind` | Required payload | Main producer / consumer |
| ---- | -------------- | ---------------- | ------------------------ |
| `INSIGHT` | `task.plan` | non-empty `tasks[]`; each task has `taskId`, `title` | planner → task ledger |
| `SIGNAL` | `task.ready` | `taskId` | CLI/reactor → task dispatcher |
| `SIGNAL` | `task.status` | `taskId`, `status` | runtime → task ledger |
| `VERIFICATION` | `task.completed` | `taskId` | runtime/reviewer → task ledger |
| `SIGNAL` | `energy.add` | positive `amount` | beekeeper → honey reserve |
| `SIGNAL` | `energy.consume` | positive `amount` | runtime → honey reserve |
| `SIGNAL` | `system.kill` | optional `reason` | beekeeper → reactor/tasks |
| `MUTATION` | `code.proposal.isolated` | proposal metadata; isolated workspace | builder → guard/review gate |
| `MUTATION` | `code.proposal.root` | proposal metadata; root workspace | hivewright → main guard |
| `MUTATION` | `code.proposal` | legacy alias of isolated proposal | legacy colony configs |
| `VERIFICATION` | `verification.success` | optional `taskId`, `summary` | guard → ledger/receiver |
| `VERIFICATION` | `verification.failed` | optional `taskId`, `summary` | guard → rework dispatch |
| `SIGNAL` | `session.invite` | `inviteId`, `bee`, `task`, `status` | auto-invite/operator → gateway |
| `SIGNAL` | `beekeeper.ready` | invite response fields | beekeeper → interactive session |
| `SIGNAL` | `artifact.written` | artifact `ref` and kind, singular or `artifacts[]` | bee/Console → trail consumers |

Narrative and operational `INSIGHT` kinds are catalogued separately in
[INSIGHT kinds](insight-kinds.md).

## Colony-defined kinds

Unknown kinds are accepted when their event envelope is valid. This lets a
colony define choreography contracts without changing Paseka. For example,
the starter ideation flow uses:

- `SIGNAL/feature.requested`
- `SIGNAL/feature.classified`
- `SIGNAL/spec.ready`

Their payload shape and routing belong in colony bee YAML, prompt emit
partials, or the relevant feature spec. Platform validation checks known kinds
more strictly but does not turn its built-in catalog into a global allowlist.

## Routing

A bee declares subscriptions and advisory publications in
`.paseka/bees/<role>.yaml`:

```yaml
subscribes:
  - type: SIGNAL
    kind: task.ready
    dispatch: task
publishes:
  - type: MUTATION
    kind: code.proposal.isolated
```

`dispatch: task` resolves the target bee from the task ledger.
`dispatch: direct` invokes the subscribing role from the event itself.
`paseka doctor` reports incompatible proposal workspace/kind wiring and other
important colony configuration problems. See [Bee routing](bee-routing.md).

## Deferred publishing

`paseka event emit --defer` writes a pending event when NATS is unavailable;
the runtime or `paseka event flush` publishes it later. These control kinds are
live-only and cannot be deferred:

- `system.kill`
- `energy.add`
- `energy.consume`
- `session.invite`
- `beekeeper.ready`
- `task.status`

Artifact announcements are additionally checked against the referenced trail
comb files before deferred flush.

## Authoring checklist

1. Reuse a platform kind when its semantics match.
2. Keep `traceId` stable across one Flight Trail and include `taskId` when the
   event belongs to one ledger task.
3. Declare producer `publishes` and consumer `subscribes` in bee YAML.
4. Validate sample events with `paseka event validate --stdin`.
5. Add a completion contract when successful adapter exit is not enough.
6. Document new colony-level payload fields beside the workflow that owns
   them.

## Related docs

- [Bee routing](bee-routing.md)
- [INSIGHT kinds](insight-kinds.md)
- [Task ledger](task-ledger.md)
- [Prompt templates](../guide/prompt-templates.md)
- [CLI event commands](../guide/cli.md)
