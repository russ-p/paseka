# Spec 005: Feature Ideation Flow

## Status

**Draft (colony reference).** Design locked for colony choreography and event shapes. **Phases 0–1** (soft path + platform/colony SIGNAL boundary) and the **colony** parts of Phases 3–4 (`auto_invites` grill/breakdown rules, grilling → `spec.ready`) are done.

**Platform** invite lifecycle (`session.invite` / `beekeeper.ready`, CLI/Console accept, `done_when` completion, session energy) lives in [006-human-gateway-invites.md](./006-human-gateway-invites.md) and [008-bee-routing.md](../008-bee-routing.md) §7–8 — not in this spec.

## Purpose

**Reference colony choreography** — one example flow (feature ideation) with colony-owned `SIGNAL` kinds (`feature.*`, `spec.ready`) and payload shapes. It depends on the platform Human Gateway ([006](./006-human-gateway-invites.md)) for parking interactive work; it does **not** redefine invite protocol.

Custom colonies use specialized bees, colony `SIGNAL` kinds, and `subscribes` / `auto_invites` rules — see [008-bee-routing.md](../008-bee-routing.md).

Target path:

```text
SIGNAL/feature.requested
  → Scout classify (AFK)
  → SIGNAL/feature.classified (decision=grill)
  → SIGNAL/session.invite (pending)          ← platform Human Gateway (006)
    → Beekeeper accept → SIGNAL/beekeeper.ready
  → Drone interactive grilling → docs/specs/… + SIGNAL/spec.ready
  → session.invite (breakdown) → Beekeeper accept (optional soft path: manual bee chat)
  → Drone breakdown → INSIGHT/task.plan → (optional) SIGNAL/task.ready
  → existing colony implementation (builder / guard / receiver)
```

This extends — does not replace — the short path in [005-task-ledger.md](../005-task-ledger.md) where a clear PRD already exists and Scout may emit `task.plan` directly.

## Goals

- Keep ideation **choreographed**: bees and colony rules react to `SIGNAL` kinds and payload fields; no central orchestrator (Scout publishes a decision scent, others react).
- Let Scout **classify and tag** with `payload.decision` (classification branch), not invent a premature task breakdown for vague ideas.
- Require explicit Beekeeper readiness before interactive Drone grilling (`when I am ready`) via Human Gateway invites ([006](./006-human-gateway-invites.md)).
- Persist grilling output as a **spec artifact** so the next bee (breakdown / AFK) is not blind to session transcript.
- Reuse existing Drone intents (`grilling`, `breakdown`) and ledger events (`task.plan`, `task.ready`).
- Preserve AFK implementation choreography after `task.plan` lands.
- Ship colony-default `auto_invites` rules for `feature.classified` (grill) and `spec.ready` (breakdown).

## Non-Goals (this spec)

- Do not redefine platform invite protocol, CLI/Console accept, or `done_when` mechanics — see [006](./006-human-gateway-invites.md) and [008](../008-bee-routing.md) §7–8.
- Do not add a new top-level `EventType` (stay on `SIGNAL` / `INSIGHT` / `MUTATION` / `VERIFICATION`).
- Do not make `INSIGHT` drive workflow routing (narrative only; see [009-insight-kinds.md](../009-insight-kinds.md)).
- Do not AFK-dispatch `intent=grilling` via `task.ready` (grilling is interactive-by-contract).
- Do not implement `confidence` budgets or `system.kill` here (see [999-backlog.md](../999-backlog.md)).
- Do not invent a second task ledger for grill/breakdown meta-tasks (optional later; not MVP).
- Do not require Object Store for MVP specs — a repo path under `docs/specs/` is enough.
- Do not register `feature.*` / `spec.ready` as hardcoded validators or reactor special-cases in `internal/protocol` — those are colony choreography contracts, not platform primitives.

## Current System Context

| Primitive | Location / behavior | Ideation use |
| --------- | ------------------- | ------------ |
| `feature.requested` | Colony contract; envelope-only on bus; no reactor subscribers | Entry scent for ideas / PRDs |
| Scout intents | `survey` (default), `plan`, `triage`, `classify` | `classify` routes without planning |
| Drone intents | `general`, `grilling`, `breakdown` | Grilling + breakdown already prompted |
| Reactor dispatch | `task` + `direct` → AFK `Adapter.Run()` only | Classify is AFK; grilling is not |
| Interactive sessions | `bee chat` / Console + Human Gateway invites | Soft path or invite accept ([006](./006-human-gateway-invites.md)) |
| Task ledger | `task.plan` → `task.ready` → implement | Starts after approved breakdown |
| Spec files | `docs/specs/*.md` (convention) | Durable handoff after grilling |
| Human Gateway | Platform invites / `auto_invites` / `done_when` | [006](./006-human-gateway-invites.md), [008](../008-bee-routing.md) §7–8 |

## Decisions

### 0. `decision` is a classification tag (not platform routing)

On colony events such as `feature.classified`, **`payload.decision`** is a classification decision tag — which branch the colony should take (`grill`, `plan`, `triage`, `clarify`, `reject`). It is **not**:

- bee **`subscribes`** dispatch (reactor AFK run selection by `type` + `payload.kind`);
- the glossary **Flight Route** (NATS subject under `events.<EventType>[.<kind>]`).

Colony **`auto_invites`** rules may **match** on `decision` (e.g. `match.decision: grill`) to publish a `session.invite` — that matching is platform routing ([008](../008-bee-routing.md) §7); the payload field itself is only Scout's classification output.

**Platform routing** stays bee `subscribes` + colony `auto_invites` with `invite.done_when`. **Choreography** is bees and rules reacting to `SIGNAL` kinds and payload fields — Scout emits `feature.classified` with a `decision`; invite rules, other bees, and Beekeeper react.

### 1. Two entry paths after `feature.requested`

| Scout `decision` | When | Next |
| ------------- | ---- | ---- |
| `grill` | Idea / vague product ask; acceptance criteria missing | `session.invite` → Drone `grilling` ([006](./006-human-gateway-invites.md)) |
| `plan` | Spec/PRD already clear enough for vertical slices | Scout (or Drone) may emit `task.plan` (existing short path) |
| `triage` | Looks like bug / debt / incident | Scout `triage` intent; not this flow |
| `clarify` | Ambiguous whether feature vs bug | `INSIGHT/context.note` + optional invite with `intent` unset; Beekeeper chooses |
| `reject` | Out of scope / duplicate / non-actionable | Narrative insight only; no invite |

Scout **must not** emit `task.plan` or `task.ready` when `decision=grill`.

### 2. Workflow uses SIGNAL; memory uses INSIGHT

| Kind | Type | Drives choreography? |
| ---- | ---- | -------------------- |
| `feature.requested` | `SIGNAL` | yes (Scout classify via `subscribes`) |
| `feature.classified` | `SIGNAL` | yes (`decision` tag → `auto_invites` / UI) |
| `session.invite` | `SIGNAL` | yes — platform Human Gateway ([006](./006-human-gateway-invites.md)) |
| `beekeeper.ready` | `SIGNAL` | yes — platform ([006](./006-human-gateway-invites.md)) |
| `spec.ready` | `SIGNAL` | yes (artifact handoff → breakdown invite or AFK breakdown) |
| `run.summary` / `context.note` | `INSIGHT` | no (prompt memory + timeline) |
| `task.plan` | `INSIGHT` | ledger only (existing) |
| `task.ready` | `SIGNAL` | yes (existing AFK work queue via `subscribes`) |

### 3. HITL grilling parks on Human Gateway invites

Interactive work is never auto-started from classify alone. This colony publishes (or auto-invites) a pending `session.invite`; Beekeeper accept starts the Drone grilling session. Invite parking-lot mechanics, accept/reject, energy, and completion are defined in [006-human-gateway-invites.md](./006-human-gateway-invites.md) — not duplicated here.

### 4. Spec artifact is the grilling completion contract

**Artifact handoff pattern (general):** a bee writes a durable file in the repo, then emits a colony `SIGNAL` with `ref` pointing at the repo-relative path so the next bee is not blind to session output. `spec.ready` is **one colony kind** of that pattern in this flow; other choreographies may use different kinds and paths.

After grilling reaches shared understanding, Drone **must**:

1. Write (or update) `docs/specs/<NNN>-<slug>.md` in the colony repo (or trace worktree if one exists — prefer colony root for committed specs).
2. Emit `SIGNAL/spec.ready` with `ref` = repo-relative path.
3. Optionally emit `INSIGHT/context.note` summarizing the Bloom for `{{.Insights}}`.

Without (1)+(2), breakdown must not start. Default grill `auto_invites` use `done_when` on `spec.ready` + file at `ref` ([008](../008-bee-routing.md) §8).

### 5. Breakdown may be interactive or AFK

| Mode | When | How |
| ---- | ---- | --- |
| Interactive (preferred) | Beekeeper wants to quiz slice granularity | Second `session.invite` with `intent=breakdown` + `artifactRef` (default `auto_invites` on `spec.ready`) |
| AFK | Spec is crisp; Beekeeper skips quiz | `paseka bee run drone --intent breakdown --task "…"` after accept, or future direct dispatch on `spec.ready` with Beekeeper opt-in |

Breakdown still follows [drone-intent-breakdown](../../.paseka/prompts/_partials/drone-intent-breakdown.md): one `INSIGHT/task.plan`, `task.ready` only when Beekeeper confirms immediate start.

### 6. Intent × mode binding

| Bee | Intent | Mode |
| --- | ------ | ---- |
| `drone` | `grilling` | **interactive only** |
| `drone` | `breakdown` | interactive preferred; AFK allowed |
| `scout` | `classify` | AFK (`direct` on `feature.requested`) |
| `builder` | `feature` / … | AFK via `task.ready` (unchanged) |

### 7. Platform vs colony SIGNAL boundary

Runtime and `internal/protocol` own **platform** kinds ([006](./006-human-gateway-invites.md)): `session.invite`, `beekeeper.ready`, plus existing ledger / energy / verification payloads.

**Colony** kinds (`feature.requested`, `feature.classified`, `spec.ready`) are choreography contracts:

| Kind | Owner | Schema source | In `internal/protocol`? |
| ---- | ----- | ------------- | ------------------------- |
| `feature.requested`, `feature.classified`, `spec.ready` | Colony / Scout / Drone prompts | This spec + emit partials | **No** — publish as ordinary `SIGNAL`; bees and docs own field shapes |
| `session.invite`, `beekeeper.ready` | Platform Human Gateway | [006](./006-human-gateway-invites.md) | Yes, at invite boundary |
| `task.*`, `energy.*`, … | Platform / ledger / reactor | Existing protocol + docs | Yes (already) |

`paseka signal` and the bus accept colony kinds with envelope checks only. Field validation for colony ideation kinds is **not** enforced by runtime — prompts and Beekeeper review are the gate. `auto_invites` reads colony event JSON via declarative rules without promoting those kinds into core protocol vocabulary.

### 8. Soft bootstrap (Phase 0)

Until invite→session is wired (or when Beekeeper prefers manual control), Beekeeper may:

```bash
paseka signal --type SIGNAL --trace "$TRACE" \
  --payload '{"kind":"feature.requested","title":"…","body":"…"}'
paseka bee run scout --intent classify --trace "$TRACE" --task "Classify the feature.requested on this trail"
paseka bee chat drone --intent grilling --trace "$TRACE" "Grill: …"
# write docs/specs/… then:
paseka bee chat drone --intent breakdown --trace "$TRACE" "Break down docs/specs/…"
```

Phase 0 still uses the colony event shapes below when emitting by hand so later phases stay compatible. Soft path remains valid even after Human Gateway invites ship.

## Event payloads

Colony-owned kinds only. Platform invite payloads (`session.invite`, `beekeeper.ready`): [006-human-gateway-invites.md](./006-human-gateway-invites.md).

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
    "decision": "grill",
    "confidence": 0.86,
    "rationale": "Product idea without acceptance criteria; needs grilling before breakdown."
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `decision` | yes | **Classification tag**: `grill` \| `plan` \| `triage` \| `clarify` \| `reject` — not NATS subject or bee `subscribes` routing |
| `confidence` | no | Advisory; not enforced |
| `rationale` | yes | Short human-readable reason |

Do **not** put `bee` / `intent` on this payload — `auto_invites` (or Beekeeper) react to `decision` and supply invite bee/intent from rule defaults.


### `SIGNAL/spec.ready`

```json
{
  "traceId": "trace-…",
  "agentId": "…",
  "type": "SIGNAL",
  "payload": {
    "kind": "spec.ready",
    "ref": "docs/specs/004-live-bees-indicator.md",
    "title": "Live bees indicator"
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `ref` | yes | Repo-relative path to the written spec |
| `title` | no | Display title |

Who runs next is **not** on this payload — colony `auto_invites` (or Beekeeper) react to `spec.ready` itself.

## Bee and prompt changes

### Scout

- Intent vocabulary entry `classify` (partial `scout-intent-classify.md`).
- Subscribe (when routing lands):

```yaml
# .paseka/bees/scout.yaml
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
- Grilling completion: write `docs/specs/…` + emit `spec.ready` (emit partial / system prompt).
- Breakdown: require `artifactRef` or readable spec path in `{{.Task}}` / Insights; keep existing `task.plan` emit rules.
- No AFK `subscribes` on `feature.requested` (avoids skipping Beekeeper).

### Colony `auto_invites` (this flow)

Platform schema and behavior: [008-bee-routing.md](../008-bee-routing.md) §7–8 and [006](./006-human-gateway-invites.md). This colony’s reference rules (grill + breakdown):

```yaml
# .paseka/colony.yaml — feature ideation reference
auto_invites:
  - when:
      type: SIGNAL
      kind: feature.classified
    match:
      decision: grill
    invite:
      bee: { default: drone }
      intent: { default: grilling }
      task:
        from_trace_kind: feature.requested
        from_trace_field: title
        prefix: "Grill feature: "
        fallback_from: rationale
        default: Grill feature
      status: pending
      done_when:
        when: { type: SIGNAL, kind: spec.ready }
        require_file: { from: ref }
        set_artifact_ref: { from: ref }
    dedupe: [bee, intent]
  - when:
      type: SIGNAL
      kind: spec.ready
    invite:
      bee: { default: drone }
      intent: { default: breakdown }
      artifactRef: { from: ref }
      task: { from: ref, prefix: "Break down ", default: Break down spec }
      status: pending
    dedupe: [intent, artifactRef]
```

With **empty** `auto_invites`, classified events do **not** create invites (no Go hardcode). Soft path (Phase 0) still works via manual `bee chat`.

## Surfaces (colony-relevant)

Platform CLI/Console invite UX: [006](./006-human-gateway-invites.md). For this flow:

| Surface | Colony behavior |
| ------- | --------------- |
| Inject idea | Form or `paseka signal` / Console event inject → `feature.requested` |
| Pending grill invite | After `feature.classified` (`decision=grill`) when grill `auto_invites` rule is present |
| Spec link | On `spec.ready`, timeline shows `ref`; breakdown invite from second rule |
| Soft path | Phase 0 manual `bee chat` commands (this spec) |

## Phased delivery

| Phase | Scope | Done when |
| ----- | ----- | --------- |
| **0 Soft** | Docs + Scout `classify` prompt + Drone grilling emit guidance for `spec.ready` | Beekeeper can run Phase 0 commands end-to-end by hand **(done)** |
| **1 Boundary** | Document platform vs colony SIGNAL ownership; keep `feature.*` / `spec.ready` out of protocol | Colony ideation kinds stay out of `internal/protocol` **(done)** |
| **2 Invites** | *Platform* — persist invites; CLI/Console accept; validate HITL kinds | See [006](./006-human-gateway-invites.md) **(done there)** |
| **3 Auto-invite** | Colony grill `auto_invites` rule on `feature.classified` | Classify → pending grill invite while `paseka run` is up **(done)** |
| **4 Hardening** | Colony: grilling → `spec.ready` + file verify via `done_when`; default `spec.ready` → breakdown auto-invite | Failed grilling without spec visible as `incomplete`; breakdown invite auto-created **(done)** |

## End-to-end scenario (happy path)

1. Beekeeper publishes `SIGNAL/feature.requested` with title/body; new `traceId`.
2. Scout AFK `classify` runs (`direct` or manual `bee run`).
3. Scout publishes `SIGNAL/feature.classified` (`decision=grill`).
4. Colony `auto_invites` → pending `session.invite` for Drone grilling ([006](./006-human-gateway-invites.md)).
5. Beekeeper later accepts → interactive Drone grilling session on the same `traceId`.
6. Drone interviews one question at a time; Beekeeper answers until shared understanding.
7. Drone writes `docs/specs/NNN-….md`, emits `SIGNAL/spec.ready` + optional `context.note`.
8. Grill invite completes via `done_when`; breakdown invite auto-created (or soft-path `bee chat` / AFK breakdown) with `artifactRef`.
9. Drone publishes one `INSIGHT/task.plan`; optionally first `SIGNAL/task.ready` if asked to start now.
10. `paseka run` implements slices via existing builder/guard/receiver choreography through final review gate.

## Anti-patterns

- Scout emitting `task.plan` for a vague idea (`decision` should have been `grill`).
- Reactor calling `Adapter.Run()` for `grilling`.
- Using `INSIGHT` kinds to trigger the next bee.
- Starting grilling automatically on `feature.classified` without Beekeeper accept.
- Running breakdown without a readable `spec.ready.ref`.
- Introducing a central “ideation orchestrator” process that sequences bees by role name.
- Emitting `bee` / `intent` / `next` on colony classification or artifact events — Scout tags with `decision`; `auto_invites` owns who is invited.
- Hardcoding `feature.*` / `spec.ready` in `internal/protocol` or reactor dispatch (colony contracts belong in prompts + this spec).
- Hardcoding auto-invite choreography in Go (use `auto_invites` in `colony.yaml`).
- Duplicating invite field tables / parking-lot mechanics in colony specs (link [006](./006-human-gateway-invites.md)).

## Open questions

- After `decision=plan`, does Scout emit `task.plan` itself or invite Drone `breakdown` without grilling?
- Should other colonies reuse `spec.ready` as a generic artifact-ready kind, or keep it ideation-specific?
- Does accept always detach in Console, or offer attached Ghostty / in-browser xterm only? (platform; track in [006](./006-human-gateway-invites.md) / Console specs)

## Related docs

- [001-brief.md](../001-brief.md) — HITL as Human Gateway, not blocking orchestrator
- [005-task-ledger.md](../005-task-ledger.md) — ledger lifecycle after `task.plan`
- [006-interactive-sessions.md](../006-interactive-sessions.md) — session runtime path
- [006-human-gateway-invites.md](./006-human-gateway-invites.md) — platform invites, accept, `done_when`, energy
- [008-bee-routing.md](../008-bee-routing.md) — `subscribes`, `auto_invites` §7, `done_when` §8
- [009-insight-kinds.md](../009-insight-kinds.md) — INSIGHT vs SIGNAL routing rule
- [010-bee-config.md](../010-bee-config.md) — bee YAML, intents
- [002-queen-console-mvp.md](./002-queen-console-mvp.md) — Sessions UI to extend with invites

## Verification

Colony soft path + classify / grilling prompts (Phases 0–1):

```bash
# Phase 0 soft path (manual)
paseka signal … feature.requested
paseka bee run scout --intent classify --trace "$TRACE" …
paseka bee chat drone --intent grilling --trace "$TRACE" "…"
# after docs/specs/… + spec.ready:
paseka bee chat drone --intent breakdown --trace "$TRACE" "…"
```

Colony auto-invite rules (Phases 3–4) with `paseka run` up:

```bash
paseka signal … feature.requested
paseka bee run scout --intent classify --trace "$TRACE" …
paseka invite list   # pending grilling invite when grill auto_invites rule is present
```

Platform invite package / protocol tests: see [006](./006-human-gateway-invites.md).
