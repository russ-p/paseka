# Spec 011: Flight trail title (`trace.title`)

## Status

**Implemented**

Operational `INSIGHT` kind for human-readable Flight Trail names in Queen Console and planner prompts.

## Problem Statement

Queen Console and trace lists show only generated `traceId` values. Beekeepers cannot scan recent trails at a glance. Task titles exist per nectar slice, but there is no trace-level display name unless the operator remembers ids.

## Solution

Introduce `INSIGHT/trace.title` — an operational, non-routing insight that sets or updates the human name of a flight trail. Runtime and Console resolve the display title with deterministic fallbacks. Planner bees (Scout intake, Drone breakdown) emit `trace.title` when they classify or plan work.

Bee-language prompts say **Flight trail title**; the wire kind stays **`trace.title`** (technical, consistent with `traceId` and other payload kinds).

## User Stories

- As a beekeeper, I see a short title on dashboard and trace lists instead of only `trace-…` ids.
- As a planner bee, I can set or refine the trail title while emitting `task.plan`.
- As a downstream bee, I see the resolved title in my prompt context via `{{.TraceTitle}}`.

## Design

### Kind shape

```json
{
  "traceId": "trace-auth-01",
  "agentId": "scout-1",
  "type": "INSIGHT",
  "payload": {
    "kind": "trace.title",
    "title": "Live bees in Queen Console header"
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `kind` | yes | `trace.title` |
| `title` | yes | Non-empty after trim; max 120 characters |

No `summary` field in v1. List subtitles remain time / task / bee metadata.

### Semantics

- **Last-write-wins** per trace (latest event by `createdAt`, then `seq`).
- **Not projected** into `{{.Insights}}` (operational, like `task.plan`).
- **Does not drive routing** or completion contracts.
- Runtime does **not** auto-emit `trace.title`; fallbacks are virtual at read time.

### Resolve order

1. Latest `INSIGHT/trace.title`
2. Latest `SIGNAL/feature.requested` → `payload.title`
3. First non-empty task title from `.paseka/runs/<traceId>/tasks/*/task.md` (sorted task ids)
4. Empty → UI uses `traceId` as primary label

### Prompt context

`prompts.Context` gains `TraceTitle string`, resolved before template render in AFK dispatch and interactive sessions.

### Queen Console

`TraceSummaryView` gains `title`. Lists and trace detail show `title` as primary line; when set, `traceId` appears muted underneath.

### Planner emit guidance

Scout intake and Drone breakdown emit `trace.title` when classifying or publishing `task.plan` (use entry/spec title or a clearer short name). Also emit on `grill` / `reject` when a short label helps Console.

## Non-goals

- Separate summarizer bee or evolving narrative summary field (see follow-up [012-trace-summary](012-trace-summary.md)).
- Bee-language wire kinds (`trail.*`).
- Required completion contract or runtime auto-synthesis of `trace.title`.

## Related docs

- [INSIGHT kinds](../reference/insight-kinds.md)
- [Prompt templates](../guide/prompt-templates.md)
- [Queen Console MVP](002-queen-console-mvp.md)
- [Feature ideation flow](005-feature-ideation-flow.md)
- [Flight trail summary](012-trace-summary.md)
