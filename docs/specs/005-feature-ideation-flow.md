# Spec 005: Feature Ideation Flow

## Status

**Draft.** Design locked for choreography and event shapes. **Phase 0 Soft** (Scout `classify` + Drone grilling `spec.ready` prompts) is implemented; Beekeeper can run the manual soft path via CLI. Phases 1–4 (protocol validation, invites, auto-invite, hardening) remain open.

## Purpose

Define how a raw feature idea becomes a durable specification and then a task ledger plan **without** a central orchestrator and **without** short-circuiting Human-in-the-Loop grilling.

Target path:

```text
SIGNAL/feature.requested
  → Scout classify (AFK)
  → SIGNAL/feature.classified (route=grill)
  → SIGNAL/session.invite (pending)
  → Beekeeper accept → SIGNAL/beekeeper.ready
  → Drone interactive grilling → docs/specs/… + SIGNAL/spec.ready
  → session.invite (breakdown) → Beekeeper accept (optional soft path: manual bee chat)
  → Drone breakdown → INSIGHT/task.plan → (optional) SIGNAL/task.ready
  → existing colony implementation (builder / guard / receiver)
```

This extends — does not replace — the short path in [005-task-ledger.md](../005-task-ledger.md) where a clear PRD already exists and Scout may emit `task.plan` directly.

## Goals

- Keep ideation **choreographed**: bees react to `SIGNAL` kinds; no FeatureOrchestrator service.
- Let Scout **classify and route**, not invent a premature task breakdown for vague ideas.
- Require explicit Beekeeper readiness before interactive Drone grilling (`when I am ready`).
- Persist grilling output as a **spec artifact** so the next bee (breakdown / AFK) is not blind to session transcript.
- Reuse existing Drone intents (`grilling`, `breakdown`) and ledger events (`task.plan`, `task.ready`).
- Preserve AFK implementation choreography after `task.plan` lands.

## Non-Goals (this spec)

- Do not add a new top-level `EventType` (stay on `SIGNAL` / `INSIGHT` / `MUTATION` / `VERIFICATION`).
- Do not make `INSIGHT` drive workflow routing (narrative only; see [009-insight-kinds.md](../009-insight-kinds.md)).
- Do not AFK-dispatch `intent=grilling` via `task.ready` (grilling is interactive-by-contract).
- Do not implement `confidence` budgets or `system.kill` here (see [999-backlog.md](../999-backlog.md)).
- Do not invent a second task ledger for grill/breakdown meta-tasks (optional later; not MVP).
- Do not require Object Store for MVP specs — a repo path under `docs/specs/` is enough.
- Do not redesign Queen Console Sessions beyond invite accept / list pending invites.

## Current System Context

| Primitive | Location / behavior | Ideation use |
| --------- | ------------------- | ------------ |
| `feature.requested` | Allowed in `protocol` validate; no subscribers | Entry scent for ideas / PRDs |
| Scout intents | `survey` (default), `plan`, `triage` | Need `classify` for routing without planning |
| Drone intents | `general`, `grilling`, `breakdown` | Grilling + breakdown already prompted |
| Reactor dispatch | `task` + `direct` → AFK `Adapter.Run()` only | Cannot start interactive sessions today |
| Interactive sessions | `bee chat` / Console `StartDetached` | Parallel path; must bind to invite accept |
| Task ledger | `task.plan` → `task.ready` → implement | Starts after approved breakdown |
| Spec files | `docs/specs/*.md` (convention) | Durable handoff after grilling |
| Human Gateway | Queen Console + CLI proposals | Needs invite queue surface |

## Decisions

### 1. Two entry paths after `feature.requested`

| Scout `route` | When | Next |
| ------------- | ---- | ---- |
| `grill` | Idea / vague product ask; acceptance criteria missing | `session.invite` → Drone `grilling` |
| `plan` | Spec/PRD already clear enough for vertical slices | Scout (or Drone) may emit `task.plan` (existing short path) |
| `triage` | Looks like bug / debt / incident | Scout `triage` intent; not this flow |
| `clarify` | Ambiguous whether feature vs bug | `INSIGHT/context.note` + optional invite with `intent` unset; Beekeeper chooses |
| `reject` | Out of scope / duplicate / non-actionable | Narrative insight only; no invite |

Scout **must not** emit `task.plan` or `task.ready` when `route=grill`.

### 2. Workflow uses SIGNAL; memory uses INSIGHT

| Kind | Type | Drives routing? |
| ---- | ---- | --------------- |
| `feature.requested` | `SIGNAL` | yes (Scout classify) |
| `feature.classified` | `SIGNAL` | yes (invite publisher / UI) |
| `session.invite` | `SIGNAL` | yes (Human Gateway; not AFK dispatch) |
| `beekeeper.ready` | `SIGNAL` | yes (session start) |
| `spec.ready` | `SIGNAL` | yes (breakdown invite or AFK breakdown) |
| `run.summary` / `context.note` | `INSIGHT` | no (prompt memory + timeline) |
| `task.plan` | `INSIGHT` | ledger only (existing) |
| `task.ready` | `SIGNAL` | yes (existing AFK work queue) |

### 3. Invite is the HITL parking lot

Interactive work is never auto-started from classify alone.

1. Something publishes `SIGNAL/session.invite` with `status: pending`.
2. Beekeeper accepts via CLI or Queen Console.
3. Accept publishes `SIGNAL/beekeeper.ready` (`action: accept`) referencing `inviteId`.
4. Runtime (or Console, same-process) starts a **detached or attached** interactive session with the invite’s `bee`, `intent`, `traceId`, and `task` text.

Reject / defer:

```json
{ "kind": "beekeeper.ready", "inviteId": "inv-001", "action": "reject" }
```

Updates invite to `cancelled` (or `deferred`); no session.

### 4. No `dispatch: session` AFK path in MVP

Do **not** teach the reactor to block on a PTY inside `paseka run`.

MVP bridge:

- Reactor (or a thin invite projector) records pending invites (filesystem and/or KV).
- Queen Console / CLI accept calls existing `sessions.Manager.StartDetached` / `bee chat`.
- Later optional: `dispatch: invite` subscription mode that only upserts invite state and never calls `Adapter.Run()`.

### 5. Spec artifact is the grilling completion contract

After grilling reaches shared understanding, Drone **must**:

1. Write (or update) `docs/specs/<NNN>-<slug>.md` in the colony repo (or trace worktree if one exists — prefer colony root for committed specs).
2. Emit `SIGNAL/spec.ready` with `ref` = repo-relative path.
3. Optionally emit `INSIGHT/context.note` summarizing the Bloom for `{{.Insights}}`.

Without (1)+(2), breakdown must not start.

### 6. Breakdown may be interactive or AFK

| Mode | When | How |
| ---- | ---- | --- |
| Interactive (preferred) | Beekeeper wants to quiz slice granularity | Second `session.invite` with `intent=breakdown` + `specRef` |
| AFK | Spec is crisp; Beekeeper skips quiz | `paseka bee run drone --intent breakdown --task "…"` after accept, or future direct dispatch on `spec.ready` with Beekeeper opt-in |

Breakdown still follows [drone-intent-breakdown](../../.paseka/prompts/_partials/drone-intent-breakdown.md): one `INSIGHT/task.plan`, `task.ready` only when Beekeeper confirms immediate start.

### 7. Intent × mode binding

| Bee | Intent | Mode |
| --- | ------ | ---- |
| `drone` | `grilling` | **interactive only** |
| `drone` | `breakdown` | interactive preferred; AFK allowed |
| `scout` | `classify` | AFK (`direct` on `feature.requested`) |
| `builder` | `feature` / … | AFK via `task.ready` (unchanged) |

### 8. Soft bootstrap (Phase 0)

Until invite→session is wired, Beekeeper may:

```bash
paseka signal --type SIGNAL --trace "$TRACE" \
  --payload '{"kind":"feature.requested","title":"…","body":"…"}'
paseka bee run scout --intent classify --trace "$TRACE" --task "Classify the feature.requested on this trail"
paseka bee chat drone --intent grilling --trace "$TRACE" "Grill: …"
# write docs/specs/… then:
paseka bee chat drone --intent breakdown --trace "$TRACE" "Break down docs/specs/…"
```

Phase 0 still uses the event shapes below when emitting by hand so later phases stay compatible.

## Event payloads

### `SIGNAL/feature.requested`

```json
{
  "traceId": "trace-…",
  "agentId": "beekeeper",
  "type": "SIGNAL",
  "payload": {
    "kind": "feature.requested",
    "title": "Live bees in Queen Console header",
    "body": "Show active bees in the console header, AFK vs session.",
    "source": "beekeeper",
    "priority": "medium"
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `kind` | yes | `feature.requested` |
| `title` | yes | Short bloom title |
| `body` | yes | Free-form idea text |
| `source` | no | `beekeeper` \| `import` \| … |
| `priority` | no | Advisory only in MVP |

### `SIGNAL/feature.classified`

```json
{
  "traceId": "trace-…",
  "agentId": "…",
  "type": "SIGNAL",
  "payload": {
    "kind": "feature.classified",
    "route": "grill",
    "bee": "drone",
    "intent": "grilling",
    "confidence": 0.86,
    "rationale": "Product idea without acceptance criteria; needs grilling before breakdown."
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `route` | yes | `grill` \| `plan` \| `triage` \| `clarify` \| `reject` |
| `bee` | when route needs a next bee | e.g. `drone` |
| `intent` | when route needs intent | e.g. `grilling` |
| `confidence` | no | Advisory; not enforced |
| `rationale` | yes | Short human-readable reason |

### `SIGNAL/session.invite`

```json
{
  "traceId": "trace-…",
  "agentId": "runtime",
  "type": "SIGNAL",
  "payload": {
    "kind": "session.invite",
    "inviteId": "inv-001",
    "bee": "drone",
    "intent": "grilling",
    "task": "Grill feature: Live bees header…",
    "status": "pending",
    "specRef": ""
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `inviteId` | yes | Stable id within the trace |
| `bee` | yes | Role to launch |
| `intent` | no | Passed to session prompt |
| `task` | yes | Initial user/task text for the session |
| `status` | yes | `pending` \| `accepted` \| `cancelled` \| `completed` |
| `specRef` | no | Set for breakdown invites |

### `SIGNAL/beekeeper.ready`

```json
{
  "traceId": "trace-…",
  "agentId": "beekeeper",
  "type": "SIGNAL",
  "payload": {
    "kind": "beekeeper.ready",
    "inviteId": "inv-001",
    "action": "accept"
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `inviteId` | yes | Must match a pending invite |
| `action` | yes | `accept` \| `reject` \| `defer` |

### `SIGNAL/spec.ready`

```json
{
  "traceId": "trace-…",
  "agentId": "…",
  "type": "SIGNAL",
  "payload": {
    "kind": "spec.ready",
    "ref": "docs/specs/004-live-bees-indicator.md",
    "title": "Live bees indicator",
    "next": { "bee": "drone", "intent": "breakdown" }
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `ref` | yes | Repo-relative path to the written spec |
| `title` | no | Display title |
| `next` | no | Suggested next bee/intent for invite publisher |

## Bee and prompt changes

### Scout

- Add intent vocabulary entry `classify` (partial `scout-intent-classify.md`).
- Subscribe (when routing lands):

```yaml
# .paseka/bees/scout.yaml (future)
subscribes:
  - type: SIGNAL
    kind: feature.requested
    dispatch: direct
default_intent: classify   # or keep survey default; classify only on this subscription
```

- Classify partial rules:
  - Prefer evidence from `body` + prior insights.
  - Emit **one** `feature.classified`.
  - Optionally `INSIGHT/run.summary`.
  - Never `task.plan` / `task.ready` when recommending `grill`.

### Drone

- Keep `grilling` / `breakdown` partials.
- Grilling completion: instruct write `docs/specs/…` + emit `spec.ready` (new emit partial or extend system prompt).
- Breakdown: require `specRef` or readable spec path in `{{.Task}}` / Insights; keep existing `task.plan` emit rules.
- No AFK `subscribes` on `feature.requested` (avoids skipping Beekeeper).

### Invite publisher (runtime)

Minimal behavior after `feature.classified` with `route=grill`:

1. Allocate `inviteId`.
2. Publish `session.invite` (`pending`) with bee/intent/task derived from classification + original idea body.
3. Project invite for Console/CLI.

After `spec.ready`:

1. Publish breakdown `session.invite` **or** surface a Console action “Start breakdown” that publishes the invite (prefer explicit Beekeeper click in MVP).

## Queen Console / CLI surfaces

| Surface | MVP behavior |
| ------- | ------------ |
| Inject idea | Form or reuse `paseka signal` / Console event inject → `feature.requested` |
| Pending invites | List `status=pending` invites for the colony/trace |
| Accept | Publish `beekeeper.ready` + start detached session (existing Sessions API) |
| Spec link | On `spec.ready`, show `ref` and offer “Start breakdown” |
| Soft path | Document Phase 0 manual `bee chat` commands in CLI help / this spec |

Suggested CLI (implementation phase):

```bash
paseka invite list [--trace <id>]
paseka invite accept <inviteId>
paseka invite reject <inviteId>
```

## Phased delivery

| Phase | Scope | Done when |
| ----- | ----- | --------- |
| **0 Soft** | Docs + Scout `classify` prompt + Drone grilling emit guidance for `spec.ready` | Beekeeper can run Phase 0 commands end-to-end by hand **(done)** |
| **1 Protocol** | Validate + document payload schemas for new SIGNAL kinds | `paseka signal` / `event emit` accept structured payloads |
| **2 Invites** | Persist pending invites; CLI `invite *`; Console list/accept | Accept starts Drone grilling session on the same `traceId` |
| **3 Auto-invite** | Publisher on `feature.classified` / optional on `spec.ready` | Classify → pending invite without manual SIGNAL crafting |
| **4 Hardening** | Completion checks for grilling (`spec.ready` required); energy policy for sessions | Failed grilling without spec is visible as failed/incomplete invite |

## End-to-end scenario (happy path)

1. Beekeeper publishes `SIGNAL/feature.requested` with title/body; new `traceId`.
2. Scout AFK `classify` runs (`direct` or manual `bee run`).
3. Scout publishes `SIGNAL/feature.classified` (`route=grill`, `bee=drone`, `intent=grilling`).
4. Invite publisher emits `SIGNAL/session.invite` (`pending`).
5. Beekeeper later accepts → `beekeeper.ready` + interactive Drone grilling session.
6. Drone interviews one question at a time; Beekeeper answers until shared understanding.
7. Drone writes `docs/specs/NNN-….md`, emits `SIGNAL/spec.ready` + optional `context.note`.
8. Beekeeper starts breakdown invite (or `bee chat` / AFK breakdown) with `specRef`.
9. Drone publishes one `INSIGHT/task.plan`; optionally first `SIGNAL/task.ready` if asked to start now.
10. `paseka run` implements slices via existing builder/guard/receiver choreography through final review gate.

## Anti-patterns

- Scout emitting `task.plan` for a vague idea (`route` should have been `grill`).
- Reactor calling `Adapter.Run()` for `grilling`.
- Using `INSIGHT` kinds to trigger the next bee.
- Starting grilling automatically on `feature.classified` without Beekeeper accept.
- Running breakdown without a readable `spec.ready.ref`.
- Introducing a central “ideation orchestrator” process that sequences bees by role name.

## Open questions

- Should pending invites live in JetStream KV, machine-local `state.json`, or both?
- Should `spec.ready` require the file to exist on disk at emit time (runtime verify)?
- After `route=plan`, does Scout emit `task.plan` itself or invite Drone `breakdown` without grilling?
- Does accept always detach in Console, or offer attached Ghostty / in-browser xterm only?

## Related docs

- [001-brief.md](../001-brief.md) — HITL as Human Gateway, not blocking orchestrator
- [005-task-ledger.md](../005-task-ledger.md) — ledger lifecycle after `task.plan`
- [006-interactive-sessions.md](../006-interactive-sessions.md) — session runtime path
- [008-bee-routing.md](../008-bee-routing.md) — `subscribes` / dispatch modes
- [009-insight-kinds.md](../009-insight-kinds.md) — INSIGHT vs SIGNAL routing rule
- [010-bee-config.md](../010-bee-config.md) — bee YAML, intents
- [002-queen-console-mvp.md](./002-queen-console-mvp.md) — Sessions UI to extend with invites

## Verification

Documentation-only updates: no build required.

When Phase 0–1 code lands:

```bash
gofmt -w .
go build -o paseka ./cmd/paseka
go test ./internal/protocol/... ./internal/prompts/...
```
