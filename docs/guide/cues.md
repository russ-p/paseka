# Forage Cues (cue layer)

A **cue** (bee language: **Forage Cue**) is a project-local YAML shortcut under `.paseka/cues/<id>.yaml` that publishes one bus ingress action — `emit: signal` or `emit: task` — with optional templates for payload fields. One cue definition drives Queen Shell (`paseka cue`), Queen Console (**Run cue**), and Telegram (`cue:` in `commands.custom`). Cues are shareable colony config (included in Nuc export/import).

Optional **`standing`** binds a cue to a long-lived **Standing Trail** (stable `traceId`) so recurring procedures reuse the same comb instead of minting a new trail every tick.

Design records: [spec 016](../specs/016-cue-layer.md), [spec 028](../specs/028-standing-trails.md) (identity slice). Vocabulary: [glossary](../idea/glossary.md) (Forage Cue, Standing Trail).

Related: [CLI](cli.md), [Telegram gateway](telegram-gateway.md), [colony layout](colony-layout.md), [task ledger](../reference/task-ledger.md), [feature ideation flow](../specs/005-feature-ideation-flow.md), [homelab deployment](homelab-deployment.md).

---

## 1. What cues are (and are not)

| Cues do | Cues do not |
| ------- | ----------- |
| Publish `SIGNAL` or `INSIGHT`/`SIGNAL` task plan (+ optional `task.ready`) | Run bees, start sessions, or accept invites |
| Seed optional per-cue **initial** honey on a fresh trace (`energy_budget`, or standing `stipend`) | Top up honey mid-flight (`paseka energy add`); standing cues do not yet refill remaining each tick |
| Bind a recurring procedure to a Standing Trail (`standing.trace`) | Ship a scheduler, webhook listener, or `report_to` callback |
| Share one definition across CLI, Console, Telegram | Replace raw `paseka signal` / `task create` for power users |
| Stay the publish API for timers and GitHub-style hooks (via `paseka cue run`) | Listen for HTTP, run cron, or reply to the webhook caller |

Cue success means the bus publish(es) succeeded (and optional honey seed). AFK reactions still need `paseka run` as today. Timers and inbound HTTP stay **outside** Paseka — see [§10](#10-external-timers-and-webhooks).

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

### Standing cue (recurring procedure)

```yaml
# .paseka/cues/daily-triage.yaml
description: Daily triage
emit: signal
type: SIGNAL
kind: triage.tick
standing:
  trace: trail-daily-triage
  stipend: 4
title: "{{.Title}}"
body: "{{.Body}}"
```

`paseka cue run daily-triage "tick …"` **without** `--trace` publishes on `trail-daily-triage`. An explicit `--trace` that equals that id is allowed; any other id fails closed. Non-standing cues still mint a new trail when `--trace` is omitted.

Standing is a **cue** binding, not a ledger flag. Recommended ids look like `trail-daily-triage` (hyphens, no dots). Two standing cues may not share one `standing.trace`. `energy_budget` is forbidden on a standing cue — stipend is the only honey number. A bare `standing:` key (null / empty) is rejected, not treated as a bloom cue.

Standing `emit: task` cues require `review: none` (or omit review) and the named bee must have `worktree: false`. Load/import errors name the cue, bee, and field.

This slice does **not** yet apply a per-tick stipend replace, refuse overlapping ticks, or refuse a killed trail. First successful run seeds honey from `standing.stipend`; later runs keep the existing seed (same as cue `energy_budget` on a live trail). Remaining 028 work: [spec 028](../specs/028-standing-trails.md).

### Schema (MVP)

| Field | Required | Notes |
| ----- | -------- | ----- |
| `description` | no | Shown in `cue list`, Console picker, Telegram help fallback |
| `emit` | yes | `signal` or `task` |
| `energy_budget` | no | Positive int — initial honey override on an unseeded **bloom** trail (§5). Forbidden when `standing` is set |
| `standing.trace` | with `standing` | Stable Flight Trail id used when the caller omits `--trace` / `traceId`. No spaces, path separators, `.`, `*`, or `>` (JetStream KV keys) |
| `standing.stipend` | with `standing` | Positive int — first-tick honey seed (§5). Required whenever `standing` is present |
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
| `Source` | Calling channel: `cli`, `console`, or `telegram` (scripted `cue run` is `cli` today) |
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
| `cue list` | Stable sort by id; prints id, optional standing trail id, optional description |
| `cue run` | Publishes immediately (no confirm). Prints trace id; task id when `emit: task`. Standing cues resolve `--trace` as above |

Examples:

```bash
paseka cue run feature "OAuth callback returns 500 on refresh"
paseka cue run hotfix "Fix nil deref in token refresh"
paseka cue run feature "Follow-up on same trail" --trace trace-abc123
paseka cue run daily-triage "tick $(date -I)"
```

Requires NATS (same as `paseka signal` / `task create`). See [CLI](cli.md) § `paseka cue`. systemd/cron and GitHub should invoke this command on the hive host — not a Paseka listener ([§10](#10-external-timers-and-webhooks)).

---

## 5. Honey: `energy_budget` vs colony default vs `energy add`

Three bloom mechanisms, plus standing first-tick seed:

| Mechanism | When | Effect |
| --------- | ---- | ------ |
| **`defaults.energy_budget`** in `colony.yaml` | First seed on a trace (task create, reactor ensure-seed, cue without override) | Sets initial `energyBudget` / `energyRemaining` (default `12`) |
| **Cue `energy_budget`** | Fresh **bloom** trail only (`energyBudget == 0` on snapshot) | Seeds a **smaller or custom initial** reserve via ledger `SeedEnergy` — can be less than colony default |
| **Standing `stipend`** | Fresh standing trail only (`energyBudget == 0`) | Seeds `energyBudget` / `energyRemaining` from `standing.stipend` (same `SeedEnergy` primitive). Does **not** refill remaining on later ticks yet |
| **`paseka energy add`** | Any time (live bus) | Increments `energyRemaining`; after seed also increments `energyAdded`. Does not change `energyBudget` |

Rules:

- Omit `energy_budget` on a non-standing cue → unchanged colony-default seeding.
- Cue with `energy_budget` on a **new** bloom trail → seed before publish (signal and task paths).
- Standing cue on a **new** trail → seed from `stipend` before publish.
- `cue run --trace` (or Console/Telegram `traceId`) on a trail that **already has** honey → cue `energy_budget` / standing stipend seed is **ignored** (no shrink, no re-seed). Per-tick stipend **replace** (`energy.stipend`) is not in this slice.
- Cues never emit `energy.add` — use CLI, Console, or Telegram `/energy` for top-ups.

Full ledger model: [task ledger](../reference/task-ledger.md) § Honey reserve.

---

## 6. Queen Console

Hive Dashboard and header expose **Run cue**:

1. Picker lists cue id + optional description (no emit-type badge). Standing cues also show `Standing: <traceId>`. `GET /api/cues` returns `standingTrace` when the cue declares standing.
2. Enter text (and optional trace id in the API; omit it on a standing cue to use `standing.trace`).
3. On success: toast with clickable `traceId` link to the trace view.

API (NATS required):

| Method | Path | Body |
| ------ | ---- | ---- |
| `GET` | `/api/cues` | — |
| `POST` | `/api/cues/:id/run` | `{"text":"…","traceId":"…","vars":{}}` |

`traceId` and `vars` are optional. Omitting `traceId` on a standing cue uses `standing.trace` (same as CLI). `agentId` / `source` are `console`. This API is for Queen Console on a **trusted** network ([homelab](homelab-deployment.md)); it is not a public GitHub/webhook endpoint.

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
- Preview + **Confirm** stay in the gate (same as `/task` and inline `emit: signal`). Standing cues show the standing trail id and confirm publishing on that trail (not “a new trace”).
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

---

## 10. External timers and webhooks

Colony YAML says **what** to publish (`.paseka/cues/<id>.yaml`). The apiary says **when** and **from where**. Paseka does not ship an HTTP webhook receiver, a cron dispatcher, or delivery back to the caller (`report_to`). Telegram `mode: webhook` is bot transport ([Telegram gateway](telegram-gateway.md)), not generic ingress.

| Layer | Owns | Lives in |
| ----- | ---- | -------- |
| Cue definition | Mnemonic + payload templates | `.paseka/cues/` (git, Nuc) |
| Schedule / hook | systemd timer, GitHub Actions, a small signed-webhook wrapper | Machine / CI — not the colony repo |
| Hive liveness | JetStream + reactor | `paseka run`; probe with `paseka status --check` |

**Cue success ≠ bees running.** `paseka cue run` publishes even if the reactor is down. A timer that must start work should probe first (`paseka status --check`), then run the cue. See [CLI](cli.md) (`paseka status --check`, `paseka cue`).

**Do not** point GitHub (or any internet webhook) at `POST /api/cues/:id/run`. Use SSH or a self-hosted runner on the hive host and call Queen Shell. Map provider JSON to cue `Text` / `--set` in the wrapper — cue files stay GitHub-agnostic (no stdin / `--file` on `cue run`).

**Idempotency** is the wrapper’s job. On a **non-standing** cue, omitting `--trace` always starts a **new** trail; a retried GitHub delivery or a double timer tick otherwise burns a second honey reserve. Remember `delivery_id` or `job@slot` and skip the second `cue run`. Recurring procedures should declare **`standing`** so omitted `--trace` reuses the procedure identity (overlap refuse is not in this slice — the wrapper should still skip a second tick).

Examples (apiary-local; adjust `-C` / paths):

```bash
# systemd / cron: probe hive, then tick a standing procedure
paseka status --check -C /path/to/colony && \
  paseka cue run daily-triage "tick $(date -I)" -C /path/to/colony

# bloom hotfix from CI on a self-hosted runner next to the hive (new trail each run)
paseka cue run hotfix "$ISSUE_TITLE" -C "$COLONY_ROOT"
```

Scheduled cues still need an active `paseka run` (and NATS) for AFK bees. Put schedules in machine config, not in committed `.paseka/`.
