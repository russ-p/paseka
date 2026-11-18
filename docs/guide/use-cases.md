# Use cases: the reference colony

> This repository is its own first user: the [`.paseka/`](../../.paseka/) directory at the repo root is a complete colony that builds Paseka with Paseka. The four walkthroughs below are that colony's recurring situations, assembled from committed configuration only — every role, cue, prompt template, and policy named here is a file you can read.

Each section is one situation end to end: how it starts, which events carry it, where a human decides, and how it lands. Field-level rules live elsewhere — [bee config](bee-config.md), [bee routing](../reference/bee-routing.md), [task ledger](../reference/task-ledger.md) — and the vocabulary is in the [glossary](../idea/glossary.md).

## 1. The cast

All bees in this colony use the `cursor` adapter; `model_aliases` in [`colony.yaml`](../../.paseka/colony.yaml) map a profile name to a model. Every role carries exactly one skill, `paseka-event-emit` (§5).

| Role | Config | Profile | Job in these flows |
| ---- | ------ | ------- | ------------------ |
| Scout | [`bees/scout.yaml`](../../.paseka/bees/scout.yaml) | classifier | Intake: classifies a `feature.requested` entry and picks the branch |
| Drone | [`bees/drone.yaml`](../../.paseka/bees/drone.yaml) | thinker | `grilling` (HITL design interview) and `breakdown` (spec → slices) |
| Builder | [`bees/builder.yaml`](../../.paseka/bees/builder.yaml) | builder | Implements every slice, hotfix, and refactor in an isolated worktree |
| Guard | [`bees/guard.yaml`](../../.paseka/bees/guard.yaml) | reviewer | Reviews each isolated proposal against the task |
| Receiver | [`bees/receiver.yaml`](../../.paseka/bees/receiver.yaml) | builder | Commits accepted work, publishes `task.completed` |
| Trail Guard | [`bees/trail-guard.yaml`](../../.paseka/bees/trail-guard.yaml) | reviewer | The closing round: reviews the whole trail before the merge gate |
| Nurse | [`bees/nurse.yaml`](../../.paseka/bees/nurse.yaml) | builder | One hygiene tick of the refactor Standing Trail (§4) |
| Archivist | [`bees/archivist.yaml`](../../.paseka/bees/archivist.yaml) | thinker | Reads finished trails; proposes skills and audits (§5) |

Three more roles exist outside these flows: Hivewright edits colony config at the repo root, Main Guard reviews those root proposals, and Probe checks adapter health.

## 2. Simple dev cycle: one hotfix

The smallest complete loop: one task, one worktree, one review, one merge. It is the inner cycle every other flow reuses.

```bash
paseka run                                        # reactor, if not already up
paseka cue run hotfix "Fix nil deref in token refresh"
```

The [`hotfix` cue](../../.paseka/cues/hotfix.yaml) is a task cue in its minimal shape: a task for Builder with `intent: bugfix`, `review: none`, `autorun: true`, and a small `energy_budget: 3`. The single command publishes the plan and readiness on a fresh trail and seeds three honey.

```text
 cue `hotfix` ──► INSIGHT/task.plan + SIGNAL/task.ready (3 honey seeded)
                        │
                        ▼  task dispatch
                  Builder ── isolated worktree .paseka/worktrees/<traceId>/
                        │    implements, stages, does NOT commit
                        ▼
         runtime mechanical gate: sh scripts/checks.sh
              │ failed: run marked checks_failed, proposal suppressed,
              │         VERIFICATION/verification.failed ──► Builder reworks
              ▼ passed
         MUTATION/code.proposal.isolated  (+ patch record carrying the gate verdict)
                        │
                        ▼  direct dispatch
                   Guard ── audits the recorded gate verdict (never re-runs it),
                        │    reviews the diff against the acceptance criteria
         ┌──────────────┴──────────────┐
         ▼ verification.failed         ▼ verification.success
   Builder reworks (direct)       Receiver (direct)
         │                             │ commits the staged work in the worktree
         └──► back to the proposal     ▼
                         VERIFICATION/task.completed (commit SHA)
                                        ▼
              runtime ensures the merge gate `_review` (phase: merge)
                                        ▼
         Beekeeper: Console Reviews preview / `paseka proposal approve`
                                        ▼
           one merge commit: "paseka: merge trace <traceId>"
```

| Step | What happens | Decided by |
| ---- | ------------ | ---------- |
| Builder runs | Task dispatch on `SIGNAL/task.ready`; the run gets the trail worktree (branch `paseka/<traceId>`, or `INSIGHT/worktree.branch`). The prompt's standing rule: *stage, don't commit*. | [`builder.yaml`](../../.paseka/bees/builder.yaml) `subscribes`, `worktree: true`; [`prompts/builder.md`](../../.paseka/prompts/builder.md) |
| Mechanical gate | After the adapter exits and **before** the proposal leaves the run, the runtime runs [`scripts/checks.sh`](../../scripts/checks.sh) from the colony checkout — never from the worktree under review, so a diff cannot weaken the gate that judges it. Go formatting, lint, module build and tests, plus the console UI checks when the diff touches `web/`. A failure marks the run `checks_failed`, suppresses the proposal, and emits `verification.failed` straight back to Builder. | [`builder.yaml`](../../.paseka/bees/builder.yaml) `checks:` block; [spec 046](../specs/046-bee-check-gate.md) |
| Guard reviews | Direct dispatch on the `MUTATION`. Guard audits the gate verdict recorded on the proposal — the newest `*.json` under `proposals/`, full output in `checks.log` — and spends its turn on requirements, scope, and design instead of re-running CI. Its completion contract demands exactly one `verification.success` or `verification.failed`, so a silent review is a failed run. | [`guard.yaml`](../../.paseka/bees/guard.yaml); [`prompts/guard.md`](../../.paseka/prompts/guard.md) |
| Rework | `verification.failed` reaches Builder as a direct dispatch. The rework cycle keys by event identity, so each Builder → Guard pass can run again on the same task. | [bee routing](../reference/bee-routing.md) § dedupe |
| Receiver commits | On `verification.success`, Receiver commits the staged work in the worktree with a conventional message and publishes `VERIFICATION/task.completed` carrying the commit SHA. | [`receiver.yaml`](../../.paseka/bees/receiver.yaml); [`prompts/receiver.md`](../../.paseka/prompts/receiver.md) |
| Merge gate | The runtime ensures one merge task `_review` (`phase: merge`, `review: final`, title from [`colony.yaml`](../../.paseka/colony.yaml)) because the trail holds an isolated proposal. The closing round is skipped — one work task is below `closing.when.min_work_tasks: 2` — and one `context.note` says so. | [task ledger](../reference/task-ledger.md) § merge gate |
| Approve | The Beekeeper previews the accumulated worktree diff in Console **Reviews** (or `paseka trail show`) and approves: `paseka proposal approve --trace <id> --task _review`. The trail branch merges into the colony root as one commit, subject `paseka: merge trace <traceId>`. Reject with `--comments-file` plans a rework task instead; discard closes without merging. | [CLI](cli.md) § `paseka proposal` |

Honey bounds the loop: every dispatch spends from the trail reserve, and an exhausted trail pauses instead of half-running.

## 3. A full feature: Scout → Drone → dev cycle → Trail Guard

The reference ideation flow ([spec 005](../specs/005-feature-ideation-flow.md)) for work that needs a design conversation before code: classify, grill, write a spec, break it into slices, run the §2 cycle per slice, close the trail as a whole.

```text
 cue `feature` ──► SIGNAL/feature.requested ──► Scout (intake, direct)
                                                  │ SIGNAL/feature.classified decision=grill
                                                  ▼
                         auto_invite: pending session.invite (Drone / grilling)
                                                  │ paseka invite accept <id> --attach (1 honey)
                                                  ▼
            Drone grilling (HITL): one question at a time, recommended answers,
            looks facts up in the codebase, design decisions go to the human
                                                  │ shared understanding
                                                  ▼
                       writes docs/specs/NNN-slug.md + SIGNAL/spec.ready (ref=path)
                                                  │ invite completes via done_when (file must exist)
                                                  ▼
                         auto_invite: pending invite (Drone / breakdown)
                          (AFK alternative: cue `spec-plan`)
                                                  ▼
            Drone breakdown: one INSIGHT/task.plan — vertical slices, blockers first
                                                  │ paseka task start --trace <id> --task 001
                                                  ▼
  ┌─ per slice: the §2 dev cycle (Builder → gate → Guard → Receiver) ─┐
  │        the ledger unlocks each dependent as its blocker completes  │
  └──────────────────── all work tasks completed ─────────────────────┘
                                                  ▼
                     closing phase: Trail Guard rounds (max 2, colony policy)
                     whole-trail audit · fix-now tasks hold the merge gate ·
                     pr.body · trace.summary · next-steps.md artifact
                                                  ▼
                    merge gate `_review` — "Review and merge: <title>"
                                                  ▼
                 Beekeeper approve ──► one merge commit (or a PR)
```

Walkthrough:

1. **Entry.** `paseka cue run feature "Webhook delivery for energy events"` publishes `SIGNAL/feature.requested` on a new trail ([`feature` cue](../../.paseka/cues/feature.yaml)).
2. **Scout intake.** Scout runs read-only (`worktree: false`) and classifies the entry. The branch is Scout's to pick: `decision: grill` — the path below; `decision: plan` or `triage` — Scout emits a one-slice `task.plan` itself; an entry that explicitly says "do now" — `task.ready` immediately. A vague plan is forbidden: every finding carries evidence (path, symbol, symptom).
3. **The grill invite.** The first [`auto_invites`](../reference/bee-routing.md#7-colony-auto_invites-human-gateway) rule in `colony.yaml` matches `feature.classified` with `decision: grill` and publishes a pending `session.invite` for Drone/grilling — task "Grill feature: <title>", `done_when: spec.ready` with a file-existence check on `ref`.
4. **The Beekeeper accepts.** `paseka invite list` → `paseka invite accept <inviteId> --attach`. Accept consumes one honey, publishes `beekeeper.ready`, and starts an interactive session on the same trail (attach later with `paseka session attach`, or in the Console terminal).
5. **Grilling (HITL).** Drone interviews one question at a time — asking several at once is bewildering — with a recommended answer for each. Facts that live in the codebase it looks up itself; the decisions are the human's. It enacts nothing until shared understanding. ([`_partials/drone-intent-grilling.md`](../../.paseka/prompts/_partials/drone-intent-grilling.md))
6. **The spec.** Drone writes `docs/specs/<NNN>-<slug>.md` (next free number) and emits `SIGNAL/spec.ready` with `ref` set to the repo-relative path. The invite's `done_when` fires — the file must exist at `ref` — and the invite turns `completed`.
7. **The breakdown invite.** The second `auto_invites` rule matches `spec.ready` and publishes a pending invite for Drone/breakdown with the spec path as `artifactRef`. The AFK alternative is the [`spec-plan` cue](../../.paseka/cues/spec-plan.yaml): `paseka cue run spec-plan "Break down docs/specs/NNN-x.md"` — the same intent as a task, publishing exactly one plan and stopping.
8. **Breakdown.** Drone reads the spec and drafts **vertical slices** (tracer bullets): each slice cuts through all layers end to end and is demoable on its own, labeled HITL or AFK (prefer AFK), blockers ordered first. One `INSIGHT/task.plan` carries the whole array. The AFK run never emits `task.ready` and never adds a review/merge task — the colony closing policy and the runtime merge gate own trail review.
9. **Kickoff.** `paseka task start --trace <id> --task 001` (omit `--task` to start every eligible slice). The ledger unlocks each dependent when its blocker completes.
10. **Per slice: the §2 cycle.** Rework happens inside a slice; the next slice starts when the ledger unlocks it.
11. **Closing phase** ([spec 045](../specs/045-trail-closing-phase.md)). When every work task is completed and the colony predicates match — `min_work_tasks: 2` and an isolated proposal on the trail — the runtime seeds `closing-1` for Trail Guard, up to `max_rounds: 2`. Trail Guard sees the **whole** accumulated change set, not one diff: acceptance coverage per task, cross-slice coherence, rework closure, delivery hygiene, verification integrity. It files at most five fix-now tasks, each `dependsOn` the closing round — that dependency is what holds the merge gate — and writes `INSIGHT/pr.body`, `INSIGHT/trace.summary`, and the `next-steps.md` artifact for everything that wants a Beekeeper decision or is merely nice to have. Follow-ups run as ordinary work; the next round reviews them. An empty round is a normal result.
12. **The merge gate.** The runtime ensures `_review` — "Review and merge: <title>" — and approval waits for every work and closing task. The Beekeeper previews the accumulated merge diff and approves, or requests changes (Console **Request changes**, or `paseka proposal reject --comments-file` — both plan a rework task and keep the gate open), or discards. With `defaults.delivery: pull_request` the same approval publishes a PR instead of merging ([PR delivery](pull-request-delivery.md)); this colony merges locally.

The human touches this flow exactly three times: accept the grill, answer the questions, approve the merge. Everything between is choreography.

## 4. Standing trail: continuous refactoring (Nurse + static checks)

Complexity debt is paid by a standing procedure, not a big-bang sweep: **one tick = one target = one bloom**.

The debt is measurable. [`.golangci.yml`](../../.golangci.yml) carries architecture limits — `cyclop` 10, `gocognit` 15, `funlen` 60 lines / 40 statements, `maintidx` 20, `interfacebloat` 10 — deliberately **not** in `enable`. `scripts/checks.sh` surfaces them in a warn-only pass, so existing debt never fails a build, and [`scripts/refactor-targets.py`](../../scripts/refactor-targets.py) prints the same report as JSON: counts per linter and candidates sorted cheapest-first, each one function (or interface) with file, line, symbol, and cost.

The procedure is the [`refactor-tick` cue](../../.paseka/cues/refactor-tick.yaml) bound to a **Standing Trail** ([spec 028](../specs/028-standing-trails.md)):

```yaml
standing:
  trace: trail-complexity
  stipend: 3      # one dispatch per tick plus room for a retried tick
```

Paseka ships no scheduler — tick it by hand or from systemd/cron ([AGENTS.md](../../AGENTS.md) documents the same command):

```bash
paseka cue run refactor-tick "tick $(date -I)"
```

A second tick is refused while the previous one is still open, so an overlapping timer cannot dispatch two refactors at once. The spawned bloom has its own `energy_budget: 4` and never spends the standing ration.

**The tick (Nurse).** Nurse runs on the colony root (`worktree: false`) — a Standing Trail observes and records; the change belongs to the bloom trail, whose Builder owns the worktree, the mechanical gate, and the merge. The tick ([`prompts/nurse.md`](../../.paseka/prompts/nurse.md)):

1. Reads `checkpoint.json` from the standing comb — `.paseka/runs/trail-complexity/artifacts/`, the procedure memory that survives between ticks: baseline counts, the last tick, the last 50 attempts, and the skip list. Written atomically (temp file, then replace).
2. Collects the report: `python3 scripts/refactor-targets.py`.
3. Takes the **first** candidate that passes three rules — not on the skip list; no attempt for the same file + symbol in the last 24 hours (yesterday's bloom may still be open); at most two attempts for the same file + symbol at the same value. A third identical attempt moves the target to `skip` with `no improvement after three attempts` and continues down the list. Nurse never picks a "more interesting" target: the report order is the procedure.
4. No eligible candidate → one idle note (`refactor tick idle candidates=0 total=…`), no cue, stop.
5. Candidate found → dispatches exactly one bloom, never with `--trace`, so the bloom mints its own trail:

   ```bash
   paseka cue run refactor "internal/x.go:42 Foo — cyclop 11 > 10"
   ```

6. Records the tick: appends the attempt to the checkpoint, one line to `journal/<date>.md`, and publishes exactly one `INSIGHT/context.note` — its completion contract, because a tick that does not record itself is a tick nobody can audit.

**The bloom ([`refactor` cue](../../.paseka/cues/refactor.yaml)).** The cue body carries the whole contract: change exactly the named finding — one function or one interface, nothing tidied or renamed beside it; the finding must be gone from the advisory report afterwards and any extracted helper must stay under the limits; behaviour unchanged; land as **one commit**; if it cannot be cleared in one focused change set, stop and report instead of widening the diff. Then the ordinary §2 machinery runs: Builder in a worktree, the mechanical gate, Guard, Receiver, the merge gate — and one small commit lands.

Why this shape:

- **Identity.** Every tick reuses `trail-complexity`, so the procedure has one history instead of a new trail per tick.
- **Memory.** The comb keeps the checkpoint between ticks; the runtime replaces remaining honey with the stipend each tick.
- **Isolation.** The standing trail can never become a merge candidate: all mutations happen on bloom trails, and `paseka doctor` warns if a standing tick bee ever publishes an isolated proposal.

One caveat worth repeating: `paseka purge --runs` deletes standing checkpoints with the rest of `.paseka/runs/`. Facts the colony must keep belong in git (`docs/`, the tracker), not in the comb.

## 5. Skills evolution: the Archivist

The newest flow, partially shipped: the colony turns what a finished trail taught it into a reusable procedure — and a human decides whether it becomes one.

Three pieces ship today: the Archivist role, the skill store with the per-role gate ([spec 041](../specs/041-skills-layer.md), [colony skills](skills.md)), and the proposal → human → store flow. One live skill exists, and every bee in this colony carries it.

**The trigger.** Archivist has no `subscribes` — manual launch only, which is also the safety property: nothing auto-spends analysis honey on every finished trail. The Beekeeper runs it on a trail worth studying:

```bash
paseka bee run archivist --trace trace-abc123 --intent skill \
  --body "Propose at most one reusable procedure from this trail."
```

The automatic alternative — subscribing a role to `SIGNAL/trail.work.completed` with `purpose: analysis` — exists; this colony has not wired it, because a retrospective after every flight trains the Beekeeper to ignore retrospectives ([self-improving colonies in practice](self-improving-colonies.md) §6).

**What the `skill` intent does** ([`prompts/archivist.md`](../../.paseka/prompts/archivist.md)):

1. Collects the trail **offline**: a jq collector over `.paseka/runs/<traceId>/` — one row per run (duration, tokens, state), task states, verdicts, lines deferred-in-spec, review notes, and a merge-lag timeline. No NATS, no `paseka export`.
2. Reads the prose the numbers cannot give: every run's `summary.md`.
3. Proposes **at most one** reusable procedure — recurring, procedural, and not already written down. A trail that offers none says so and lets the completion contract fail rather than publishing a fabricated proposal; that failure is the truthful result ("no entry"), not a broken colony.
4. Writes the bundle to the trail comb: `artifacts/skill-proposals/<name>/` with `SKILL.md` (frontmatter `name` + `description`) and optional `scripts/`, `references/`, `assets/`. It never writes `.paseka/skills/` itself — nothing installs itself.
5. Publishes exactly one `INSIGHT/skill.proposed`.

**The human half.** Approval and activation are separate acts, and that separation is the whole safety story:

```bash
paseka skills proposals                    # pending proposals, bundle paths
paseka skills approve trace-abc123/<name>  # installs into .paseka/skills/ — nothing is committed
# activation is a separate, reviewed edit — the per-role gate is the bee's own list:
#   .paseka/bees/builder.yaml → skills: [paseka-event-emit, <name>]
paseka skills validate && paseka doctor
git commit                                 # store + role list land in one reviewed commit
```

Or refuse: `paseka skills reject trace-abc123/<name> --reason "…"` — terminal for that exact bundle; a changed bundle with a new hash is a new proposal.

**The living example.** [`paseka-event-emit`](../../.paseka/skills/paseka-event-emit/SKILL.md) — the procedure every bee in this colony uses to publish a bus event and prove it landed — began exactly this way. Its frontmatter still carries the provenance: `source: proposal`, `role: archivist`, `trail: trace-01a1da38673703e4`.

**What is still TBD** — described, not yet wired:

- an automatic retrospective reaction (the "steward" pattern above);
- a lesson journal and a promotion-or-refuse policy — methodology, not runtime ([the idea](../idea/self-improving-colony.md), [the applied loop](self-improving-colonies.md));
- fork probes and control arms to test a candidate procedure before promoting it;
- skill revisions — a proposal whose name is already installed ([spec 053](../specs/053-skill-revisions.md), draft).

## 6. Where to go next

| Goal | Doc |
| ---- | --- |
| The cue layer behind every entry point above | [Forage Cues](cues.md) |
| Task vs direct dispatch, rework dedupe, `auto_invites` | [Bee routing](../reference/bee-routing.md) |
| Review policies, phases, the ensured merge gate | [Task ledger](../reference/task-ledger.md) |
| The mechanical check gate | [Spec 046](../specs/046-bee-check-gate.md), [AGENTS.md](../../AGENTS.md) § After Go changes |
| The closing phase and trail review | [Spec 045](../specs/045-trail-closing-phase.md) |
| Standing Trails, stipends, checkpoints | [Spec 028](../specs/028-standing-trails.md), [Forage Cues](cues.md) § standing |
| The skills store and the per-role gate | [Colony skills](skills.md) |
| The full self-improvement loop this flow feeds | [Self-improving colonies in practice](self-improving-colonies.md) |
| Ideation flow design record | [Spec 005](../specs/005-feature-ideation-flow.md) |
