# 034 — Swarm Mode (best-of-N and model comparison)

**Status:** Draft
**Supersedes:** —
**Related:** [008 code proposal workspaces](008-code-proposal-workspaces.md),
[016 cue layer](016-cue-layer.md), [019 model aliases](019-model-aliases.md),
[027 config profiles](027-config-profiles.md), [024 pull-request delivery](024-pull-request-delivery.md),
[042 trail fork](042-trail-fork.md),
[043 trail observability](043-trail-observability.md),
[idea: self-improving colonies](../idea/self-improving-colony.md)

---

## 1. Problem

A `task.ready` dispatch runs exactly one bee once. If the approach is wrong,
a human notices during review and creates rework. There is no first-class way
to say "try three approaches and show me the best one" — and there is no way
to compare model families, providers, or thinking levels on real colony work
rather than synthetic benchmarks.

Two distinct needs hide behind this:

1. **Best-of-N** — raise the odds that at least one variant is a good
   solution, chosen by a reviewer (human or judge bee).
2. **Observability polyglot** — collect fact-based comparison data
   (tokens, exit, guard pass/fail, diff size, wall time) across
   `adapter` × `model` × `thinking` on the same task.

Both are blocked today by the same gap: `params.model` / `bee.adapter` are
statically bound in bee YAML and resolved once per dispatch. Neither the cue
layer nor `task.plan` can override them per run, and the Worktree Manager
keys worktrees by `traceId` only.

## 2. Goals

- A task may declare a **swarm** of N variants. Each variant is an
  independent adapter run on the same task, in its own worktree.
- Variants may differ in `params.model`, `params.thinking`, `params.provider`,
  and (opt-in) `adapter`.
- One worktree per variant under `.paseka/worktrees/<traceId>/<variantId>/`.
- Runtime emits one `SIGNAL/swarm.ready` when all variants have completed.
- A judge bee or a human selects the winner. Winner follows the existing
  isolated-proposal review path; losers are discarded.
- All variants produce the same run artifacts as today (`prompt.txt`,
  `system.txt`, `summary.md`, `meta.json`, `status.json`, `events.ndjson`,
  `result.json`) so existing readers keep working.
- A read-only **stats** surface aggregates run artifacts for model comparison.

## 3. Non-goals (v1)

- Automatic winner selection without a human or a judge bee. A colony may
  configure a judge bee, but the default path is human review.
- PR delivery for losers. Under `defaults.delivery: pull_request`, only the
  winner publishes a head.
- Reroll of losers as a first-class CLI verb (can be done by re-running the
  same cue with a new `swarm.pick`).
- Cross-trace swarms. A swarm is scoped to one `traceId` and one task.
- Standing cues. `swarm` on a standing cue is rejected at load, to keep
  standing ticks cheap and predictable.
- Model routing / cost optimization as a runtime concern. Swarm produces
  the data; choosing a model is still a colony-author decision.

## 4. Design

### 4.1 Two layers of configuration

**Bee YAML — policy.** What a role is willing to run and with what default
variants.

```yaml
# .paseka/bees/builder.yaml
swarm:
  max_count: 5
  allow_adapter_override: false       # default; set true to mix CLI adapters
  sample: 1.0                         # 0.0–1.0; fraction of matching tasks
  shadow: false                       # true → run variants, gate on primary only
  variants:
    - { id: cheap,  adapter: opencode, model: local/qwen2.5-coder-32b }
    - { id: cloud,  adapter: cursor,   model: medium }
    - { id: strong, adapter: claude,   model: high, thinking: high }
  primary: cloud                      # which variant drives the gate if shadow
```

`primary` is required when `shadow: true`. Variants without an explicit
`id` get one derived from their index (`v1`, `v2`, …).

**Cue / task.plan — request.** Which subset of the policy to run for this
dispatch.

```yaml
# .paseka/cues/feature-swarm.yaml
emit: task
bee: builder
intent: feature
review: required
swarm:
  pick: [cheap, cloud, strong]   # subset of bee.swarm.variants by id
  # optional override of the allowed adapter mix for this cue:
  allow_adapter_override: true
```

`pick` selects by variant `id` from the bee policy. Unknown ids fail the
cue load with a message naming the bee, the cue, and the missing id.
`pick` length must be ≤ `swarm.max_count`.

### 4.2 Variant overlay in `ResolveBee`

The runtime resolves an **effective bee per variant** by starting from the
effective bee (after any config profile is applied) and overlaying the
variant's `adapter` and `params`:

```go
// internal/colony/bee.go (sketch)
func (b Bee) WithVariant(v VariantSpec) (Bee, error) {
    if v.Adapter != "" && !b.Swarm.AllowAdapterOverride {
        return Bee{}, ErrAdapterOverrideNotAllowed
    }
    out := b
    if v.Adapter != "" {
        out.Adapter = v.Adapter
    }
    out.Params = mergeParams(b.Params, v.Params)
    return out, nil
}
```

This is a **runtime-efemeral** overlay, not a committed `*.local.yaml` and
not a config profile. It does not affect routing, completion contracts,
prompt templates, or publishes — only `adapter` and `params`.

Model aliases (`colony.yaml` `model_aliases`, overlayed by home
`config.yaml`) resolve normally inside the overlay. A variant may set
`model` to an alias (`high`) or a raw vendor id.

### 4.3 Dispatch flow

```
task.ready (swarm.pick = [cheap, cloud, strong])
        │
        ▼
Reactor resolves []VariantSpec from effective bee
        │
        ▼
For each variant: create worktree <traceId>/<variantId>,
                  dispatch via the standard Prepare → Run → Finalize path
        │
        ▼
N × MUTATION/code.proposal.isolated with payload.variant = <variantId>
        │
        ▼
All N completed → runtime emits SIGNAL/swarm.ready once
        │
        ▼
Judge bee or human → VERIFICATION/swarm.winner { variantId }
        │
        ▼
Runtime merges winner worktree, removes losers → task.completed
```

The dispatch path is unchanged: each variant uses the same `agentId`
allocation, the same `.paseka/runs/<traceId>/<agentId>/` layout, the same
adapter registry, the same completion contracts, the same run summary
policy. Swarm is a fan-out over the existing path, not a new path.

### 4.4 Worktree layout change

Today the Worktree Manager keys worktrees by `traceId`:

```
.paseka/worktrees/<traceId>/
```

Swarm extends this to a sub-key, keeping single-variant trails unchanged:

```
.paseka/worktrees/<traceId>/             # non-swarm trail (unchanged)
.paseka/worktrees/<traceId>/<variantId>/ # swarm trail
```

`homestate/state.json` gains a `variants[]` array on the worktree entry:

```json
{
  "traceId": "trace-a1b2c3d4e5f6a7b8",
  "branch": "paseka/trace-a1b2c3d4e5f6a7b8",
  "baseSha": "…",
  "variants": [
    { "variantId": "cheap",  "branch": "paseka/trace-…/cheap",  "worktreePath": ".paseka/worktrees/trace-…/cheap" },
    { "variantId": "cloud",  "branch": "paseka/trace-…/cloud",  "worktreePath": ".paseka/worktrees/trace-…/cloud" },
    { "variantId": "strong", "branch": "paseka/trace-…/strong", "worktreePath": ".paseka/worktrees/trace-…/strong" }
  ]
}
```

Branch naming: `paseka/<traceId>/<variantId>`. Existing rules for
`INSIGHT/worktree.branch` apply per variant, but the default is now
variant-scoped. A single-variant trail keeps `paseka/<traceId>`.

Direct dispatch workspace affinity (isolated → trace worktree, root →
colony root) must select the right variant sub-tree when the trigger carries
`payload.variant`.

### 4.5 Deduplication key

The direct-dispatch dedupe key today is
`traceId + taskId + bee + type + kind`. With N variants this collapses to
one run. Add `variantId` (empty for non-swarm runs):

```
traceId + taskId + bee + variantId + type + kind
```

Rework-cycle gates (`MUTATION/code.proposal.isolated`, `code.proposal.root`,
`VERIFICATION/verification.failed`) continue to key by event identity within
their variant.

### 4.6 Honey Reserve

Each variant consumes one token, same as today. A swarm of N needs N tokens
in the reserve before the first dispatch fires.

- **Pre-flight:** if `energyRemaining < N * budget_multiplier`, the task
  moves to `blocked` with summary
  `Swarm needs N tokens, X remaining`. No partial fan-out.
- **`budget_multiplier`** is a colony-level `defaults` field (default `1.0`).
  Use `> 1` to reserve headroom for a rework pass after judging.
- **Standing cues + swarm:** rejected at load. Standing ticks stay cheap.
- Cue `energy_budget` seeds a bloom as today; a swarm on that bloom uses
  the seeded reserve.

A `SIGNAL/energy.consume` fires per variant dispatch, as usual.

### 4.7 Judging

`SIGNAL/swarm.ready` is emitted **once by the runtime** after the last
variant reaches a terminal state (completed or failed). Payload:

```json
{
  "traceId": "trace-…",
  "agentId": "runtime",
  "type": "SIGNAL",
  "payload": {
    "kind": "swarm.ready",
    "taskId": "task-1",
    "variants": [
      { "variantId": "cheap",  "status": "completed" },
      { "variantId": "cloud",  "status": "completed" },
      { "variantId": "strong", "status": "failed"    }
    ]
  }
}
```

The judge picks by emitting:

```json
{
  "traceId": "trace-…",
  "agentId": "judge-01",
  "type": "VERIFICATION",
  "payload": { "kind": "swarm.winner", "taskId": "task-1", "variantId": "cloud" }
}
```

The runtime merges the winner's worktree (isolated path, `review: final`
semantics) and removes loser worktrees. Under
`defaults.delivery: pull_request`, only the winner publishes a head.

Failed variants are still listed in `swarm.ready`; the judge may pick a
completed variant or reject the whole swarm (see §4.10).

### 4.8 Shadow mode

When `swarm.shadow: true`:

- All variants run and write their normal artifacts.
- Only `primary` participates in the gate (`waiting_review`, merge,
  `task.completed`).
- Non-primary variants are marked `MUTATION/code.proposal.isolated` with
  `payload.shadow: true`. The runtime does **not** defer completion on them
  and does **not** dispatch reviewers to their worktrees.
- Loser worktrees are removed on trail completion, same as a normal
  non-winning variant.

Shadow is the cheap way to collect comparison data on real tasks without
changing the merge outcome.

### 4.9 Observability and `paseka swarm stats`

Every variant writes the standard run artifacts. The following fields are
the comparison surface:

| Source | Field |
| ------ | ----- |
| `meta.json` | `adapter`, `model` (resolved), `variantId`, `workspace` |
| `status.json` | `status`, `exitCode`, `finishedAt` − `startedAt` |
| `result.json` | `usage.inputTokens`, `usage.outputTokens`, `usage.cacheReadTokens`, `usage.cacheWriteTokens`, `providerSessionId` (when the adapter supplies them) |
| `events.ndjson` | `MUTATION/code.proposal.isolated` payload — diff size, files touched |
| `events.ndjson` | `VERIFICATION/verification.success` vs `failed` from the guard run |
| `VERIFICATION/swarm.winner` | Which `variantId` the judge picked |

New read-only command:

```
paseka swarm stats [--since 30d] [--group-by model|adapter|intent|sector]
                   [--intent feature] [--trace <id>]
```

It walks `.paseka/runs/*/` and aggregates. Output is a plain table; no new
bus events, no new storage. This is deliberately post-hoc — the goal is
factology on real work, not a live dashboard.

A `paseka swarm list <traceId>` shows the variants of a single trail and
their statuses.

### 4.10 Rejection and failure

- **All variants failed:** runtime emits `swarm.ready` with all `failed`
  and does **not** advance the task. The trail stays in `waiting_review`
  with a summary naming the swarm. A human decides to rework or abandon.
- **Judge picks nothing:** same as above. There is no automatic fallback
  to "first completed variant".
- **`system.kill` during a swarm:** kills all live variants on the trail.
  Their worktrees are not merged; the trail is `cancelled`.

### 4.11 Doctor checks

`paseka doctor` gains:

- **Error** — `swarm.pick` references an unknown variant id.
- **Error** — `swarm.shadow: true` without `swarm.primary`.
- **Error** — swarm declared on a standing cue.
- **Error** — `allow_adapter_override: false` but a variant sets `adapter`.
- **Warning** — swarm on a bee whose `publishes` does not include
  `MUTATION/code.proposal.isolated` (root proposals do not swarm in v1).
- **Warning** — `max_count > 5` without an explicit `budget_multiplier`.

### 4.12 Dynamic configuration (proposed amendment)

The two layers in 4.1 are both **committed**: the bee policy lives in
`.paseka/bees/<role>.yaml`, the cue request in `.paseka/cues/<id>.yaml`.
A colony therefore cannot say "for this task, try these three models" without
editing a reviewed file — and a role that wants to choose its own variants at
dispatch time has no way to express it.

The request layer is already the right place: `INSIGHT/task.plan` carries an
optional `tasks[].swarm`, and a cue is only one of the things that publish a
task plan. A planning role is another. So the amendment is not a new
mechanism — it is a second source of the same request.

**Precedence.** The effective variant set for a task is resolved in this
order, first match wins:

| # | Source | Kind |
| --- | --- | --- |
| 1 | the task plan's `tasks[].swarm` | request |
| 2 | the cue's `swarm.pick` | request |
| 3 | the bee's `swarm:` section | policy |

A request narrows the policy; it never widens it. A task plan that names a
variant the bee policy does not define is refused, and the refusal names the
bee, the task and the missing id — the same rule the cue path already uses.

**What a role may set.** The request is data from a role, so it is bounded by
policy, not trusted:

| Axis | Settable by a role | Why |
| --- | --- | --- |
| which of the policy's variants to run | yes | a choice inside the reviewed set |
| `params` (model alias, thinking level) | yes, within the variant's declared params | cost is bounded by the policy's variant list |
| `adapter` | only when `allow_adapter_override` is true | changes the client's trust boundary |
| `publishes`, `subscribes`, `completion_contract` | never | these are the role's contract, not its execution detail |

A request that tries to set a forbidden axis is refused with the axis named.
The refusal is not fatal to the dispatch: the task runs with the policy's
default set, and the refusal is recorded.

**Relation to the fork.** [Spec 042](042-trail-fork.md) forks one task with
one changed `.paseka/`; this section fans one task out over several variants.
The two compose: a fan-out whose variants each carry a config delta is the
shape a methodology needs, and the delta is applied by the fork mechanism
rather than invented here. This spec owns the fan-out; the fork spec owns the
delta.

### 4.13 Doctor checks (amended)

The checks in 4.11 gain one: **Warning** — a role that publishes
`INSIGHT/task.plan` with a `swarm` request, but whose bee configuration has
no `swarm:` policy section. Without a policy there is nothing to narrow, so
the request is refused at dispatch and the trail runs one variant instead.

## 5. Data model changes

| Store | Change |
| ----- | ------ |
| `.paseka/bees/<role>.yaml` | New optional `swarm` section (see §4.1) |
| `.paseka/cues/<id>.yaml` | New optional `swarm` section with `pick` |
| `.paseka/colony.yaml` | New optional `defaults.swarm_budget_multiplier` |
| `~/.config/paseka/<slug>/state.json` | `variants[]` on worktree entries |
| `INSIGHT/task.plan` payload | New optional `tasks[].swarm` field |
| `SIGNAL/swarm.ready` | New platform kind (see §4.7) |
| `VERIFICATION/swarm.winner` | New platform kind (see §4.7) |
| `MUTATION/code.proposal.isolated` payload | New optional `variant`, `shadow` |

No existing field changes meaning. Non-swarm trails are byte-identical to
today.

## 6. Queen Console

**Reviews.** When a task has variants, the review page shows a tab per
variant with its diff, summary, tokens, wall time, and guard verdict.
Actions:

- **Pick winner** — same merge / PR flow as `review: final` today.
- **Reject all** — moves the task to `waiting_review` with a swarm
  rejection summary; no merge.
- **Reroll** (v1 scope: opens a new cue run, not a direct button).

**Traces.** A swarm trail shows the fan-out in the topology view: one
`task.ready` node fanning to N adapter nodes, joining at `swarm.ready`.

**Stats.** A read-only table over the same aggregate as `paseka swarm stats`
— useful for the model-comparison use case, filtered by adapter and model.

## 7. CLI

```
paseka task create --bee builder --intent feature \
  --body "…" --swarm cheap,cloud,strong

paseka swarm list  <traceId>
paseka swarm stats [--since 30d] [--group-by model] [--adapter cursor]
```

`paseka task create --swarm` writes the same `task.plan` field the cue
layer writes. No new task op semantics.

## 8. Migration

- **Existing bees:** no `swarm` section → behaviour identical.
- **Existing cues:** no `swarm` section → identical.
- **Existing worktrees:** `.paseka/worktrees/<traceId>/` unchanged for
  non-swarm trails. Registry entries gain an empty `variants[]` field.
- **Existing platform kinds:** unchanged. `swarm.ready` and `swarm.winner`
  are additive; old colonies simply never see them.
- **`paseka doctor`:** the new checks are warnings for existing colonies
  that never declare `swarm`.

No breaking changes.

## 9. Open questions

1. **Pricing facts.** Cursor and Claude expose token usage; Pi and
   OpenCode are partial. Should `swarm.stats` accept a machine-local
   per-model price table, or leave cost out of v1 and report tokens only?
2. **Judge bee as a first-class role.** Ship a `judge` bee template in
   `paseka init`, or leave judging to humans and colony authors in v1?
3. **Variant-scoped `INSIGHT/worktree.branch`.** Does each variant emit its
   own branch insight, or does the runtime emit one at `swarm.ready`?
4. **Cross-task swarms.** A single swarm across multiple `taskId`s is out
   of scope; confirm it stays out.
5. **`allow_adapter_override` default.** `false` is safe but blocks the
   primary use case (comparing CLI adapters). Reconsider after the first
   colony adopts swarm.
6. **PR delivery for shadow variants.** Currently no head is pushed for
   shadow losers. Confirm that is acceptable for the comparison use case.

The dynamic configuration in 4.12 adds its own questions.

7. **Who may request a variant set.** Is a role's `tasks[].swarm` accepted
   as-is, or does it need the colony's consent for the model it names? The
   amendment bounds the request to the policy's variant list, which bounds
   cost, but a policy that lists an expensive model is still an expensive
   model. Alternative: the policy may carry a per-variant cost ceiling that
   a request cannot exceed.
8. **What a request costs.** Every variant spends the trail's honey, so a
   role that requests four variants spends four dispatches' worth of budget
   on one task. Should a request be charged against the same honey reserve,
   or against a separate experiment budget the way a fork has its own
   ([spec 042](042-trail-fork.md))?
9. **Judge visibility.** The judging section (§4.7) has the judge pick a
   winner. Harness²'s selection is **blind** — the judge sees anonymised
   deliverables and not the harness that produced them — and the reason is
   that an unblinded judge rewards the most elaborate output rather than the
   best one. Should the judge here also be blind to which variant produced
   which candidate?
10. **A variant that changes the procedure, not the model.** 4.2 overlays
    `adapter` and `params` only. A methodology would want variants that
    differ in the procedure itself — a different prompt set, a different
    skill set. That is a config delta, which is what a fork carries
    ([spec 042](042-trail-fork.md)). Should a variant be able to name a
    delta, or should a fan-out of forks be the only way to express it?
11. **Task plan ownership.** A cue publishes a task plan, and now a planning
    role may publish one with a swarm request. Does the runtime distinguish a
    cue's plan from a role's plan when the role's plan arrives later for the
    same task — last writer wins, or is a plan immutable once dispatched?
12. **Naming.** `variant` already means a reasoning level in the adapter
    layer (`internal/adapters/session.go:63`). Should the swarm concept keep
    the name, and the reasoning level take another?

## 10. References

- [008 code proposal workspaces](008-code-proposal-workspaces.md) —
  isolated vs root proposal invariants
- [016 cue layer](016-cue-layer.md) — cue schema and standing rules
- [019 model aliases](019-model-aliases.md) — alias resolution in `params.model`
- [027 config profiles](027-config-profiles.md) — why swarm is **not** a
  profile overlay: profiles are process-scoped, swarm is dispatch-scoped
- [024 pull-request delivery](024-pull-request-delivery.md) — how the
  winner publishes under `pull_request`
- [042 trail fork](042-trail-fork.md) — the config delta a variant may
  carry; a fork is the smallest unit of comparison
- [043 trail observability](043-trail-observability.md) — the per-trail
  metrics that `swarm stats` aggregates
- [idea: self-improving colonies](../idea/self-improving-colony.md) — why a
  fan-out and a fork are the same idea at two sizes
- `internal/colony/bee.go` — `Bee` struct and `LoadBee`
- `internal/runtime/reactor.go` — dispatch fan-out and dedupe
- `internal/worktree/` — worktree registry and variant sub-paths
- `internal/homestate/` — `state.json` schema
