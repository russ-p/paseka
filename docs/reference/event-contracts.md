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

Every event the colony publishes is also written to its Flight Trail's record,
`.paseka/runs/<traceId>/trail-events.ndjson`, before the bus attempt. A Bee's own
`events.ndjson` remains the per-attempt audit of what that run published; the
trail record is the whole-trail history and is what the timeline, `paseka export`,
prompt memory, trail titles and summaries, and branch and pull-request copy read.
See [Architecture overview](../architecture/overview.md) § Run tree layout.

The trail record is what the colony decided to do, not proof of what was
broadcast: a saved event may still have failed to reach the bus, and nothing on
the record says so. `paseka verify <traceId>` compares the record against the
stream and reports what is only local, only on the bus, or in both.

Each event envelope also carries an `id`, minted by the colony where the event is
created. It is what makes storing one event in two files safe — a merged read
shows it once — and it is readable in the raw envelope on a timeline row so a row
can be correlated with a bus message. It is **not** part of this contract: no
producer supplies it, it is absent from the agent-facing input shape, and no
colony author may rely on its name or meaning.

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
| `SIGNAL` | `energy.stipend` | positive `amount` | standing cue runtime → honey remaining (set, not add) |
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
[INSIGHT kinds](insight-kinds.md) (`pr.body` is operational pull-request copy;
see [pull-request delivery](../guide/pull-request-delivery.md)).

### `MUTATION` refs

A `code.proposal*` payload is valid with at least one of `diff`, `ref`, or
`summary`, and the two ref conventions are different things:

- **Archive ref** — a runtime-synthesized proposal carries `ref` = a
  colony-root-relative path into the trail's proposal store
  (`.paseka/runs/<traceId>/proposals/<agentId>.patch`) and **no** `diff`. The
  bytes live in the patch file; the Guard reviews the working tree, not this
  ref. Only the runtime writes these, and only a new synthesized event acquires
  the semantics — the shape is what distinguishes one, so a reader resolves it
  against `ColonyRoot` and never against a sector or worktree.
- **Legacy object-store ref** — a proposal published by a version that offloaded
  diffs above 64 KiB carries an opaque `ref` (an object name, not a path). It is
  never reclassified as a file: no reader turns a legacy object name into a
  filesystem path. `paseka purge --bus` still deletes those objects.

An explicitly authored `MUTATION` with an inline `diff` remains valid and is
passed through unchanged; it gets no automatic patch conversion. See
[Spec 039](../specs/039-proposal-patch-records.md).

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

## Forked trails and consumer scope

An experimental fork is an ordinary Flight Trail on the colony's shared bus. Its
events keep the normal subjects, envelope, and payloads; the only general
identity that distinguishes a fork is `traceId`. Do not look for a fork-specific
field, header, or subject prefix in v1.

Built-in work-producing and side-effecting consumers — the Reactor, interactive
sessions, the Human Gateway notifier, review/delivery, metrics, and audit reads —
resolve the event's trail lineage through the shared trail read model before
acting. `trailview.ResolveScope` reads the trail's configuration policy and fork
lineage and reports `experimental`, its `forkOf` source, and any missing
evidence. A consumer that cannot resolve scope logs the problem and does not
guess; it never falls back to treating an experiment as ordinary work.

The same rule applies in both directions:

- Console, metrics, and audit reads observe source and fork together, carrying
  `forkOf`/lineage rather than hiding the experiment.
- Work-producing and delivery consumers apply the pinned experimental policy
  before dispatch, merge, branch push, or pull-request notification. Fork events
  may run internal proposal and verification choreography, but they do not
  deliver their branch.

An independent NATS subscriber that receives the same subjects must perform the
same lineage lookup itself; a subscription alone does not implement this
contract. External side-effecting subscribers that cannot resolve lineage are
**unsupported** in v1. Paseka does not claim broker-level tenant separation or
filesystem/OS sandboxing of external agents. A transport-level fork marker (for
example a NATS header) is deferred until a concrete external consumer requires
it. See [Spec 042](../specs/042-trail-fork.md) §13.

## Deferred publishing

`paseka event emit --defer` writes a pending event when NATS is unavailable;
the runtime or `paseka event flush` publishes it later. These control kinds are
live-only and cannot be deferred:

- `system.kill`
- `energy.add`
- `energy.consume`
- `energy.stipend`
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
