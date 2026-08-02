# Forage Cues (cue layer)

A **cue** (bee language: **Forage Cue**) is a project-local YAML shortcut under `.paseka/cues/<id>.yaml` that publishes one bus ingress action — `emit: signal` or `emit: task` — with optional templates for payload fields. One cue definition drives Queen Shell (`paseka cue`), Queen Console (**Run cue**), and Telegram (`cue:` in `commands.custom`). Cues are shareable colony config (included in Nuc export/import).

Design record: [spec 016](../specs/016-cue-layer.md). Vocabulary: [glossary](../idea/glossary.md) (Forage Cue).

Related: [CLI](cli.md), [Telegram gateway](telegram-gateway.md), [colony layout](colony-layout.md), [task ledger](../reference/task-ledger.md), [feature ideation flow](../specs/005-feature-ideation-flow.md).

---

## 1. What cues are (and are not)

| Cues do | Cues do not |
| ------- | ----------- |
| Publish `SIGNAL` or `INSIGHT`/`SIGNAL` task plan (+ optional `task.ready`) | Run bees, start sessions, or accept invites |
| Seed optional per-cue **initial** honey on a fresh trace | Top up honey mid-flight (`paseka energy add`) |
| Share one definition across CLI, Console, Telegram | Replace raw `paseka signal` / `task create` for power users |

Cue success means the bus publish(es) succeeded (and optional honey seed). AFK reactions still need `paseka run` as today.

---

## 2. Layout

```
.paseka/cues/
├── feature.yaml    # signal → feature.requested (Scout intake)
└── hotfix.yaml     # task → builder bugfix autorun (small energy_budget)
```

Cue **id** = filename without extension. `paseka init` scaffolds `feature` and `hotfix` when missing.

---

## 3. Authoring

### Signal cue (`feature`)

```yaml
# .paseka/cues/feature.yaml
description: Intake an idea or bug for Scout classification
emit: signal
type: SIGNAL
kind: feature.requested
title: "{{.Title}}"
body: "{{.Body}}"
```

Publishes one `SIGNAL` on a new trace (or `--trace` / API `traceId` when attaching). Omitted `energy_budget` → colony default applies on first seed (see §5).

### Task cue (`hotfix`)

```yaml
# .paseka/cues/hotfix.yaml
description: Urgent fix via builder (small honey reserve)
emit: task
bee: builder
intent: bugfix
review: none
autorun: true
energy_budget: 3
title: "{{.Title}}"
body: "{{.Body}}"
```

Reuses `task create` semantics: `INSIGHT/task.plan`, then `SIGNAL/task.ready` when `autorun: true`.

### Schema (MVP)

| Field | Required | Notes |
| ----- | -------- | ----- |
| `description` | no | Shown in `cue list`, Console picker, Telegram help fallback |
| `emit` | yes | `signal` or `task` |
| `energy_budget` | no | Positive int — initial honey override on an unseeded trail (§5) |
| **signal** | | |
| `type` | yes | Must be `SIGNAL` |
| `kind` | yes | `payload.kind` (e.g. `feature.requested`) |
| `static` | no | Extra string fields merged into payload |
| `title`, `body` | no* | `text/template` strings (*required when templates reference them) |
| `payload_template` | no | Escape hatch: rendered string → JSON object |
| **task** | | |
| `bee`, `intent`, `review`, `autorun` | per task create | Same semantics as `paseka task create` |
| `title`, `body` | no* | Templates for task plan fields |

**Template engine:** Go `text/template` (same family as prompt templates). Built-in context:

| Key | Meaning |
| --- | ------- |
| `Text` | Operator text (positional CLI arg, Console modal, Telegram command body) |
| `Title` | First line of `Text` |
| `Body` | Full `Text` |
| `Source` | `cli`, `console`, or `telegram` |
| `TraceID` | Resolved flight trail |

Extra vars: `--set key=val` (CLI) or `vars` map (Console API). Missing template keys → error. Unused `--set` keys are ignored. Operator `Text` / `--set` wins over cue `static` defaults.

---

## 4. Queen Shell

```bash
paseka cue list [-C path]
paseka cue run <id> <text> [--trace id] [--set key=val ...] [-C path]
```

| Command | Behavior |
| ------- | -------- |
| `cue list` | Stable sort by id; prints id + optional description |
| `cue run` | Publishes immediately (no confirm). Prints trace id; task id when `emit: task` |

Examples:

```bash
paseka cue run feature "OAuth callback returns 500 on refresh"
paseka cue run hotfix "Fix nil deref in token refresh"
paseka cue run feature "Follow-up on same trail" --trace trace-abc123
```

Requires NATS (same as `paseka signal` / `task create`). See [CLI](cli.md) § `paseka cue`.

---

## 5. Honey: `energy_budget` vs colony default vs `energy add`

Three different mechanisms:

| Mechanism | When | Effect |
| --------- | ---- | ------ |
| **`defaults.energy_budget`** in `colony.yaml` | First seed on a trace (task create, reactor ensure-seed, cue without override) | Sets initial `energyBudget` / `energyRemaining` (default `12`) |
| **Cue `energy_budget`** | Fresh trail only (`energyBudget == 0` on snapshot) | Seeds a **smaller or custom initial** reserve via ledger `SeedEnergy` — can be less than colony default |
| **`paseka energy add`** | Any time (live bus) | Increments `energyRemaining` only; does not change `energyBudget` |

Rules:

- Omit `energy_budget` on a cue → unchanged colony-default seeding.
- Cue with `energy_budget` on a **new** trace → seed before publish (signal and task paths).
- `cue run --trace` (or Console/Telegram `traceId`) on a trail that **already has** honey → cue `energy_budget` is **ignored** (no shrink, no re-seed).
- Cues never emit `energy.add` — use CLI, Console, or Telegram `/energy` for top-ups.

Full ledger model: [task ledger](../reference/task-ledger.md) § Honey reserve.

---

## 6. Queen Console

Hive Dashboard and header expose **Run cue**:

1. Picker lists cue id + optional description (no emit-type badge).
2. Enter text (and optional trace id in the API).
3. On success: toast with clickable `traceId` link to the trace view.

API (NATS required):

| Method | Path | Body |
| ------ | ---- | ---- |
| `GET` | `/api/cues` | — |
| `POST` | `/api/cues/:id/run` | `{"text":"…","traceId":"…","vars":{}}` |

`traceId` and `vars` are optional. `agentId` / `source` are `console`.

---

## 7. Telegram gateway

In `~/.config/paseka/<slug>/telegram.yaml`, `commands.custom.<name>` may use **`cue: <id>`** instead of inline `emit` / `type` / `kind`:

```yaml
commands:
  custom:
    feature:
      description: "Intake idea/bug via Scout"
      cue: feature
```

- Loads `.paseka/cues/feature.yaml` from the colony repo.
- Preview + **Confirm** stay in the gate (same as `/task` and inline `emit: signal`).
- `description` in gate config wins; falls back to cue `description`.
- Inline `emit: signal` custom commands still work — migration is optional.

Reserved command names unchanged: `start`, `status`, `help`, `invites`, `traces`, `energy`, `task`.

See [Telegram gateway](telegram-gateway.md) § Custom commands.

---

## 8. Nuc packs

Cues export and import with bees and prompts:

```bash
paseka nuc export -o pack.nuc.yaml              # includes all cues when present
paseka nuc export --cues feature,hotfix -o …    # filter cue ids
paseka nuc import ./pack.nuc.yaml
```

Nuc `spec.cues` maps id → raw YAML body. Import validates cue files. Details: [Nuc packs](nuc.md).

---

## 9. Ideation entry

For the reference ideation flow, `paseka cue run feature "…"` is the everyday replacement for hand-written `SIGNAL/feature.requested`. Scout `intake` still reacts via `subscribes` when `paseka run` is active.

```bash
paseka cue run feature "Live bees indicator in Console header"
# equivalent to publishing feature.requested on a new traceId
```

See [feature ideation flow](../specs/005-feature-ideation-flow.md) § Entry paths and § Soft bootstrap.
