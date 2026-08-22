# Spec 016: Cue layer (Forage Cue)

## Status

**Implemented.** Colony ingress via `.paseka/cues/`; CLI `paseka cue list|run`, Queen Console Run cue API/UI, Telegram `cue:`, Nuc export/import with `--cues`, optional per-cue `energy_budget`. Durable docs: [cues guide](../guide/cues.md).

## Problem Statement

Beekeepers start colony choreography from three surfaces that do not share a definition: raw `paseka signal`, `paseka task create`, and Telegram `commands.custom` with inline `emit: signal`. Remembering payload shapes (`feature.requested` fields, task bee/intent/autorun) is noisy for everyday entry points like “intake an idea” or “hotfix now.” Duplicating the same slash-command recipe in docs and `telegram.yaml` drifts. Queen Console has no first-class named shortcut either — only lower-level inject/task paths.

Operators want short mnemonics (`feature`, `hotfix`) that publish the right first scent without learning emit details, without introducing a workflow DAG executor.

## Solution

Introduce a **Cue** (bee language: **Forage Cue**): a project-local YAML file under `.paseka/cues/<id>.yaml` that declares a thin ingress action — `emit: signal` or `emit: task` — and optional templates for payload fields. An optional per-cue `energy_budget` can **override the colony default initial honey reserve** for a newly seeded Flight Trail (e.g. cheaper `hotfix`), without using `energy.add`. One cue definition drives Queen Shell (`paseka cue run` / `list`), Queen Console (Dashboard/header Run cue), and Telegram (`cue:` as an alternative to inline `emit:`). The cue runtime publishes bus events and may seed ledger honey for a fresh trace; channel-specific UX (Telegram preview + Confirm) stays in the gate. Cues are shareable colony config, included in Nuc export/import.

## User Stories

1. As a Beekeeper, I want named cues in the colony repo, so that entry sequences travel with the project instead of living only on one machine.
2. As a Beekeeper, I want `paseka cue list`, so that I can see available mnemonics and optional descriptions without opening YAML.
3. As a Beekeeper, I want `paseka cue run feature "…"`, so that I can start Scout intake without hand-writing `SIGNAL/feature.requested` JSON.
4. As a Beekeeper, I want `paseka cue run hotfix "…"`, so that I can seed a builder bugfix task (plan + ready) without juggling `task create` flags.
5. As a Beekeeper, I want cue run to publish immediately on CLI (no confirm step), so that shell usage stays as fast as `signal` / `task create`.
6. As a Beekeeper, I want optional `--trace` on cue run, so that I can attach an ingress publish to an existing Flight Trail when needed.
7. As a Beekeeper, I want a new trace when `--trace` is omitted, so that phone/CLI shortcuts default to a fresh trail like Telegram custom signals today.
8. As a Beekeeper, I want `--set key=val` on cue run, so that I can override template variables without a declared args schema.
9. As a Beekeeper, I want unknown template variables to fail closed, so that a typo in a cue file does not publish a half-empty payload.
10. As a Beekeeper, I want my Text / `--set` values to win over cue `static` defaults, so that one-off overrides are predictable.
11. As a Beekeeper, I want cue run without stdin / `--file` body sources, so that the CLI surface stays one positional text plus flags.
12. As a Beekeeper using Queen Console, I want a Dashboard/header “Run cue” action, so that I can start choreography without dropping to the shell.
13. As a Beekeeper in Console, I want the cue picker to show name plus optional description only, so that the UI stays a mnemonic list, not an emit debugger.
14. As a Beekeeper after Console publish, I want a toast with a traceId link, so that I can jump to the new or targeted trail immediately.
15. As a Beekeeper on Telegram, I want `commands.custom` to reference `cue: <id>`, so that slash commands and CLI share one definition.
16. As a Beekeeper on Telegram, I want preview + Confirm to remain gate behavior, so that phone mistakes do not publish without a second tap.
17. As a Beekeeper, I want to keep existing inline `emit: signal` custom commands working, so that migration to `cue:` is optional.
18. As a Beekeeper writing a cue, I want one file per cue id (filename = id), so that git diffs and Nuc packs stay obvious.
19. As a Beekeeper writing a cue, I want optional `description` and YAML comments, so that I can document intent without a separate docs page.
20. As a Beekeeper after `paseka init`, I want a bundled `feature` cue, so that ideation intake works out of the box.
21. As a Beekeeper adopting reference patterns, I want a documented `hotfix` cue shape (task / builder / bugfix / autorun / small `energy_budget`), so that urgent fixes have a known cheap mnemonic.
22. As a Beekeeper packing a Nuc, I want cues included in export/import, so that tuned entry points move with bees and prompts.
23. As a Beekeeper packing a Nuc, I want a `--cues` filter analogous to `--bees`, so that I can ship a subset of cues.
24. As a colony author, I want cue actions limited to publish (`signal` or `task`), so that cues never become a hidden DAG or `bee run` launcher.
25. As a colony author of a signal cue, I want typed fields plus an optional escape-hatch payload template, so that simple cases stay declarative and odd shapes remain possible.
26. As a colony author of a task cue, I want up to two publishes (`task.plan` and optional `task.ready`), so that autorun-style entry matches today’s `task create --autorun`.
27. As a gate operator, I want Telegram command `description` to live in gate config when convenient, with fallback to the cue description, so that localization/wording can differ per machine without forking the cue file.
28. As a Beekeeper reading docs/UI, I want the technical word `cue` in CLI/Console, so that I am not forced to learn bee metaphor to operate.
29. As a documentation reader, I want **Forage Cue** in the glossary mapped to `cue`, so that Experience Layer language stays consistent.
30. As a Beekeeper when a cue file is missing or invalid, I want a clear CLI/API/gate error naming the cue id, so that misconfiguration is obvious.
31. As a Beekeeper when NATS is down, I want cue run to fail like `signal` / `task create`, so that there is no fake success path.
32. As a Beekeeper listing cues, I want stable sort by id, so that scripts and UI order match.
33. As a Console API consumer, I want list + run endpoints for cues, so that the SPA and future clients share one contract.
34. As a Beekeeper passing empty Text where the cue requires body/title material, I want a usage error, so that blank publishes do not create empty trails.
35. As a Beekeeper using `--set` keys the template does not reference, I want those extras ignored (not failed), so that shared scripts can pass optional overrides safely.
36. As a reactor/swarm, I want cue publishes to look like ordinary bus events (same kinds/payload contracts), so that existing `subscribes` and ledger paths need no special cue type.
37. As a Beekeeper, I want `source` / `agentId` to reflect the channel (`cli`, `console`, `telegram`), so that timelines show where the scent entered.
38. As an implementer of Telegram, I want cue resolution to load project `.paseka/cues/` and only apply gate Confirm + allowlists, so that cue runtime stays channel-agnostic.
39. As a Beekeeper with reserved Telegram command names, I want the same reserved-name rules when `cue:` is used, so that `/task` and builtins cannot be shadowed.
40. As a Beekeeper writing advanced signal payloads, I want the escape-hatch template to render with the same context and fail-on-missing rules as typed fields, so that one mental model covers both paths.
41. As a Beekeeper authoring `hotfix`, I want `energy_budget: 3` (or similar) on the cue, so that the trail starts with a smaller honey reserve than `defaults.energy_budget` instead of burning a full ideation budget on a small fix.
42. As a Beekeeper omitting `energy_budget` on a cue, I want the colony default seed (`defaults.energy_budget`) to apply unchanged, so that most cues need no honey fields.
43. As a Beekeeper running a cue with `--trace` on an already seeded trail, I want `energy_budget` ignored (no shrink / no re-seed), so that cues cannot silently rewrite live honey accounting.
44. As a Beekeeper, I want cue honey override to use ledger `SeedEnergy` (initial budget), not `SIGNAL/energy.add`, so that I can set a **smaller** reserve than the colony default — which top-up cannot do.
45. As a Beekeeper on a signal cue (`feature`) with `energy_budget` set, I want the reserve seeded before the bus publish, so that the first reactor dispatch does not overwrite with the colony default.
46. As a Console/Telegram user running the same cue, I want the same `energy_budget` seeding rules as CLI, so that channel choice does not change honey economics.

## Implementation Decisions

### 1. Naming and glossary

- Technical term: **cue** (CLI verb `paseka cue`, path `.paseka/cues/`, UI label `cue`).
- Bee language: **Forage Cue** (Experience Layer). Add to the glossary branding/user tables when this ships (or when the spec is Approved, if glossary is updated ahead of code).
- Avoid `preset` (collides in perception with Nuc). Alternate names (recipe, trailhead, starter, spark) are rejected for this feature.

### 2. Ownership and layout

- Cues are **project-local only**: `.paseka/cues/<id>.yaml`.
- Cue **id** = filename without extension (e.g. `feature.yaml` → `feature`). No separate `name:` field required for MVP.
- No machine-local cue directory in MVP.
- `paseka init` creates at least `.paseka/cues/feature.yaml` aligned with ideation intake (`SIGNAL` / `feature.requested`).
- Document and prefer shipping `hotfix` as the reference task cue, e.g.:

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

- `feature` omits `energy_budget` (colony default). Shipping both `feature` and `hotfix` from `paseka init` is preferred when the init surface stays small.

### 3. Action kinds (publish only)

| `emit` | Behavior |
| ------ | -------- |
| `signal` | One bus publish: envelope type from cue (`SIGNAL` for MVP custom ingress; keep field for clarity/validation). Payload from typed fields and/or escape-hatch template. |
| `task` | Reuse task-create semantics: publish `INSIGHT/task.plan`, then optionally `SIGNAL/task.ready` when autorun is true (two publishes max). |

- No `bee run`, invite accept, multi-step graphs, or waits for bee completion inside cue execution.
- Cue success = successful publish(es) (and successful optional honey seed); AFK reactions still require `paseka run` as today.
- Timers, inbound HTTP, and GitHub-style hooks are **not** cue channels. They invoke the same Queen Shell `cue run` (or an equivalent trusted-host wrapper) that humans use. Do not add a webhook route table, cron table, or result callback into the cue runtime.

### 4. Per-cue initial honey (`energy_budget`)

- Optional integer field `energy_budget` on any cue (`signal` or `task`).
- Meaning: **initial** per-trace honey reserve for a **not-yet-seeded** trail — overrides `colony.yaml` → `defaults.energy_budget` (and platform default) for that `traceId` only.
- Implementation: call existing ledger `SeedEnergy(traceId, N)` (same primitive as task create / reactor ensure-seed). **Not** `SIGNAL/energy.add` (add only increments `energyRemaining` and cannot set a smaller budget).
- Ordering: resolve `traceId` → if cue has `energy_budget` and snapshot `energyBudget == 0`, seed → then publish signal or task plan(+ready). For `emit: task`, pass the cue budget into the shared create path so colony default is not seeded first.
- When `energy_budget` is omitted: unchanged today — task path seeds colony default; signal path leaves seed to first ensure-seed / dispatch with colony default.
- When `--trace` (or equivalent) targets a trail that already has `energyBudget > 0`: **ignore** cue `energy_budget` (do not shrink, do not re-seed, do not error solely for this mismatch). Document in CLI help.
- Validation: if present, must be a positive integer; `0` / negative → cue validation error.
- No `emit: energy`, no cue-side `energy.add`, no bus kind `energy.seed` in this MVP.
- Console picker stays name + description only (budget is an authoring detail, not a list badge).

### 5. File schema (conceptual)

Per-cue YAML supports:

- Optional `description` (string)
- Required `emit`: `signal` | `task`
- Optional `energy_budget` (positive int) — see §4
- For `signal`: `type` (must be `SIGNAL` in MVP), `kind`, optional string map `static`, optional typed `title` / `body` templates, optional escape-hatch `payload_template` (rendered string → JSON object)
- For `task`: fields aligned with task create defaults — `bee`, `intent`, `review`, `autorun`, templates for `title` / `body`
- YAML comments allowed (standard YAML)

**Template model C (hybrid):** declarative typed fields for the common path; optional escape-hatch template for non-flat payloads. Prefer typed fields for bundled `feature` / `hotfix`.

### 6. Render context and CLI args

Built-in context:

| Key | Meaning |
| --- | ------- |
| `Text` | Positional operator text (required when templates need it; empty Text → usage error if title/body would be empty) |
| `Title` | First line of `Text` (truncated to the same practical limit as Telegram custom signals today) |
| `Body` | Full `Text` |
| `Source` | Channel: `cli` \| `console` \| `telegram` |
| `TraceID` | Resolved trace (new or `--trace`) |

- Additional vars: only via `--set key=val` (CLI) or equivalent `vars` map (API/Console). No declared args schema in MVP. No stdin / `--file` body input for cue run.
- Template engine: same family as prompt rendering (`text/template`), with **missing key = error**.
- Unused `--set` keys: **ignore** (do not fail).
- Precedence: operator `Text` / derived Title/Body / `--set` **win** over cue `static` defaults (user wins).

CLI sketch:

- `paseka cue list [-C path]`
- `paseka cue run <id> <text> [--trace id] [--set key=val ...] [-C path]`
- Immediate publish (no confirm). Print trace id (and task id when `emit: task`) on success.

### 7. Shared runtime

- One library/module loads, validates, renders, seeds optional honey, and publishes cues.
- CLI, Console API, and Telegram gate all call that runtime after channel-specific auth/UX.
- Envelope `agentId` / payload `source` reflect the calling channel (`cli`, `console`, `telegram`). Scripted `cue run` uses `cli` until a later change adds an explicit machine-origin tag.
- Publishing reuses existing bus + task-create helpers; honey override reuses `SeedEnergy` — do not invent parallel ledger writers or a new energy bus kind for MVP.

### 8. Telegram gate

- Extend `commands.custom.<name>` with optional `cue: <id>` as an **alternative** to inline `emit` / `type` / `kind`.
- When `cue` is set, load `.paseka/cues/<id>.yaml` and run signal-or-task publish after Confirm; do not duplicate kind fields inline unless needed for help text.
- Keep preview + Confirm in the gate; cue runtime remains confirm-agnostic.
- `description`: prefer gate field when present; else fall back to cue `description` if that keeps validation simple.
- Preserve reserved command names and existing inline `emit: signal` path.
- Task cues from Telegram: same Confirm UX as today’s `/task` family (preview then publish); exact card copy may show cue id + description.

### 9. Queen Console

- SPA in the **same** feature ship as CLI + gate wiring (not deferred).
- Placement: Hive Dashboard / header action “Run cue”.
- Picker: cue id + optional description only (no emit-type badge required in MVP).
- On success: toast + clickable `traceId` link into the existing trace view.
- API: list cues; run cue with text + optional traceId + optional vars map.

### 10. Nuc

- Include `.paseka/cues/*.yaml` in Nuc documents (`spec.cues` or equivalent map id → body).
- `paseka nuc export --cues id,...` filter analogous to `--bees` (default: all cues when exporting cues; define interaction with bee-only exports so omit-cues vs all-cues is explicit — default **include all cues** when present unless `--cues` filters; if only `--bees` is set without cue intent, still include all cues unless an explicit `--no-cues` is added — prefer: **cues export with bees by default; `--cues` filters; empty `--cues` means none** only if that matches bees semantics — mirror `--bees`: omit flag = all, flag = filter).
- Import writes cue files with the same force/dry-run rules as bees/prompts; no field-level merge.

### 11. Docs on ship

- Guide: cue authoring + CLI; Telegram `cue:` example; Console Run cue; `energy_budget` vs colony default vs `paseka energy add`; external timers/webhooks as wrappers around `cue run`.
- Glossary: Forage Cue ↔ cue.
- Changelog entry; remove backlog cue row on ship.
- Ideation / telegram / task-ledger guides link here for ingress honey override instead of duplicating recipes.

### 12. External timers and webhooks

- Colony owns **what** (cue YAML). The machine/CI owns **when** and **from where** (systemd, GitHub Actions, a signed-webhook script). Schedules do not live under `.paseka/`.
- Queen Console `POST /api/cues/:id/run` is a trusted-network Console API, not a public webhook. Internet providers call Queen Shell on the hive host (SSH / self-hosted runner), never the Console port.
- Wrappers map provider JSON to cue `Text` / `--set`. Cue YAML must not encode GitHub/GitLab payload schemas.
- Duplicate deliveries: wrappers dedupe (`delivery_id`, timer slot). Cue run without `--trace` always allocates a new trail; the bus does not noop retries.
- `paseka status --check` is the substrate probe for timers; it is not part of cue success. Telegram gate `mode: webhook` remains bot transport ([010](010-telegram-human-gateway.md)), not this ingress.

## Testing Decisions

- Test **external behavior**: load/validate cue files; render context (Title/Body/Source/TraceID/`--set`); fail on missing template keys; user-wins precedence; signal publish payload shape; task plan+ready when autorun; `energy_budget` seeds smaller-than-default reserve on fresh traces; omitted budget keeps colony default; already-seeded `--trace` ignores cue budget; invalid `energy_budget` rejected; list sorting; Telegram config accept `cue:` vs inline emit; reserved names; API list/run contracts.
- Do not assert internal helper names or file paths beyond the stable `.paseka/cues/<id>.yaml` contract.
- Modules: cue load/validate/render unit tests; publish + seed integration with bus/ledger fakes; gate config tests extended; Console API handler tests; optional CLI smoke.
- Prior art: Telegram `commands.custom` validation and `BuildCustomSignalPayload` tests; `paseka task create` / tasks ops + `SeedEnergy` tests; Nuc export filter tests for `--bees`.

## Out of Scope

- DAG / playbook / multi-step executors beyond task plan+optional ready (+ optional honey seed)
- `bee run`, session start, invite accept from cues
- Machine-local cue overrides directory
- Declared positional/named args schema in cue YAML
- stdin / `--file` for cue body
- CLI confirm / dry-run as required MVP (optional later)
- Non-`SIGNAL` cue emits in MVP (e.g. raw `INSIGHT` ingress) except task.plan’s existing INSIGHT usage inside `emit: task`
- Cue editing UI in Console (run + list only)
- Renaming Forage Cue / changing tech term after Approval without a deliberate glossary change
- Windows-specific packaging concerns unique to cues
- Cue `energy.add` / `emit: energy` / mid-flight top-up shortcuts (keep `paseka energy add` / gate `/energy`)
- Shrinking or rewriting honey on an already seeded trail
- New bus kind to carry budget (e.g. `energy.seed`) — MVP uses ledger `SeedEnergy` only
- Per-cue override of colony `energyBudget` display semantics beyond first seed
- In-process HTTP webhook receiver or path→cue route table
- Built-in cron / scheduled cue dispatcher
- Delivering cue or bee results back to the HTTP caller (`report_to` / callback)
- Bus-level cue-run idempotency or `dedupe_key` (wrappers own GitHub delivery-id / timer slot)
- Treating Queen Console cue-run HTTP as public internet ingress
- Mapping raw webhook JSON in the cue runtime (no stdin / `--file`; wrappers render `Text` / `--set`)
- Dedicated `--source` / `agentId` values for cron or GitHub (scripted runs stay `cli` until a later spec)

## Further Notes

- Design shipped as [Forage Cues guide](../guide/cues.md).
- Complements [005 ideation](005-feature-ideation-flow.md) entry paths and [010 Telegram custom emit](010-telegram-human-gateway.md) without replacing raw `paseka signal` (power users / evals keep it).
- Honey model background: [task ledger](../reference/task-ledger.md) (`energyBudget` vs `energy.add`).
- Related durable docs after ship: [telegram gateway](../guide/telegram-gateway.md), [CLI](../guide/cli.md), [colony layout](../guide/colony-layout.md), [nuc](../guide/nuc.md), [glossary](../idea/glossary.md).
- External timers and webhooks: [Forage Cues](../guide/cues.md) § External timers and webhooks. A machine-origin `source` tag (`cron`, `github`) is optional later work if Flight Trails must distinguish scripted vs human CLI; it is not required to keep hooks outside the binary.
