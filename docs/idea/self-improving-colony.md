# Self-improving colonies

> An idea note. What a colony could learn from its own trails, where the line runs between the platform and the methodology that uses it, and which specs carry the work.

## The gap

A colony runs, fails, gets reworked, and forgets.

A trail that ended in `verification.failed`, a trail that took three Return Flights, a trail that succeeded but cost four times its class median — all of it survives as a trail: a comb, a run record, an event log. None of it becomes anything the next trail can use. The knowledge that would help ("this role checks the acceptance list before proposing code", "this guard re-reads the whole file instead of the diff") sits in the operator's head or in an unstructured prompt, where it is neither reviewable nor testable.

The platform is not missing the ability to *store* that knowledge. It is missing the ability to *test* it: to run the same task against a changed procedure and see whether the change helped. Without that, any colony-level improvement is a guess, and a guess that touches shared files is a regression waiting for a bad week.

## The source idea: Harness²

The shape of the problem and a workable answer come from [Harness²](https://github.com/google-research/harness2) (Google Cloud AI Research, Northwestern; arXiv 2610.14224). Its claim is that an agent is a model plus a **harness** — everything around the model: system prompt, guardrails, memory, tool guidance, subagents, skills, scripts, hooks — and that with the model frozen, the harness is the thing worth optimising.

Three properties of that work carry over:

- **The harness is a file tree.** A change to how an agent works is a change to files, diffable, reviewable, revertible. In Paseka the equivalent tree is `.paseka/` — prompts, bees, skills — which is already the unit a `code.proposal.root` mutation moves.
- **Two recursions.** Inside a task, the harness is refined by *Propose → Probe → Compose*: propose an edit, run the same task under the edited harness, compose a final harness from the behavioural contrast between the runs. After the task, a *Reflector* turns the round into at most one transferable lesson (`cue` / `lesson` / `evidence`) and appends it to a library that the next task's editors read.
- **The base is never silently rewritten.** Each new task starts from the base harness; only the library carries forward. The paper's ablations are the reason this matters: refining the base harness *without* probes and without a library produced results **worse than the base** (73.4 → 63.2 on one of its task sets). Blind improvement is worse than none.

Two more results shape what is worth copying. The editors are split into two orthogonal dimensions — **INV** (how the agent works: tool errors, mechanical repetition, ignored invariants) and **DOM** (what a practitioner knows: analytic moves, artifact contracts, decision rules) — and splitting them is worth several points of quality, because one editor asked to "improve everything" spreads itself thin. And the reflector is allowed to answer **"no"**: a lesson that rests only on confounded evidence is refused, and then nothing at all is written to the library, not even the harness that was adopted.

## What transfers

| Harness² | Paseka shape | Where it lands |
| --- | --- | --- |
| Harness as an editable file tree | `.paseka/` — prompts, bees, skills | already the unit of `MUTATION/code.proposal.root` |
| The eight harness components | prompt templates, per-role skills, completion contracts, the `emit` how-to | [spec 041](../specs/041-skills-layer.md) |
| Progressive disclosure: a skill costs nothing until invoked | per-role skill list, advertisement carries metadata only | spec 041, decisions 3 and 8 |
| Route each failure to the cheapest effective component | tool guidance → script → skill → contract → one always-on line | methodology content, not platform |
| Probe: same task, edited harness, behavioural contrast | **fork**: a new trail from the same base with a `.paseka/` delta | [spec 042](../specs/042-trail-fork.md) |
| Refine the harness from the contrast | dynamic variant sets, decided per task rather than committed per role | [spec 034](../specs/034-swarm-mode.md), amended |
| Reflector: one lesson, `cue` / `lesson` / `evidence`, `NO_ENTRY` | lesson records beside the skill store | methodology content, shipped as a Nuc |
| Blind selection among anonymised candidates | judge picks the winner of a fan-out without seeing which harness produced it | spec 034, judging section |
| Base stays fixed; the library carries forward | two-stage promotion: hypothesis → rule, both behind a human | methodology content |
| Noise discipline: a defect in 1 of k runs is noise | how many base and probe runs a claim is worth | methodology content |

## Two kinds of thing: platform and methodology

This is the line the rest of this note exists to draw.

**The platform provides capabilities.** It records what happened; it delivers a procedure to a role and remembers what was offered and what was reached for; it carries a proposed change as a reviewable artifact; it forks a trail; it isolates an experiment from the live colony; it spends a budget. These are mechanisms, they are the same for every colony, and they are what a spec in `docs/specs/` describes.

**The methodology provides decisions.** When is a trail worth a retrospective; which failure deserves which change; how many runs count as evidence; what "better" means; when a hypothesis becomes a rule; when a rule is rolled back. These are opinions about how to work, they change per domain, and they belong to the colony that holds them — not to the runtime.

The reason the split matters is that the second kind is where the value is and where the risk is. A methodology that lives in the platform becomes a platform feature that every colony inherits and every maintainer has to keep forever. A methodology that lives as content can be tried, replaced, and shipped by someone who is not the runtime's author.

So: **the methodology ships as a Nuc pack** — a role, its prompts, its skills, its thresholds — and is installed into a colony like any other pack. The platform does not know that it exists.

### The capability surface

A Nuc pack can only be written against a stable list of what the platform offers. That list is the contract a methodology targets:

| # | Capability | Why a methodology needs it |
| --- | --- | --- |
| R1 | A trail's metrics as one machine-readable object (`paseka inspect metrics --json`) | to pick a trail worth studying without reading raw logs |
| R2 | A "this trail is finished" signal | to start work when a trail ends instead of polling |
| R3 | The trail's artifacts, events and trajectories, exportable | to locate the moment a run went wrong |
| R4 | A proposed change as a reviewable artifact (`MUTATION/code.proposal.root` + patch record) | to propose a change without writing into the colony |
| R5 | A fork: a new trail from a chosen base with a chosen `.paseka/` delta, recorded as a commit | to test a proposed change against the same task |
| R6 | A plane: a separate bus, home and checkout for one experiment | to run an experiment beside the live colony |
| R7 | A budget per trail, with a cap | to keep an experiment from spending the colony's honey |
| R8 | A skill store with a per-role list and a deterministic load | to deliver a procedure and to know it was reached for |

R1–R3 are the [observability spec](../specs/043-trail-observability.md), which also records what the budget was spent on. R4 exists today ([spec 039](../specs/039-proposal-patch-records.md)). R5 and R6 are [spec 042](../specs/042-trail-fork.md). R8 is [spec 041](../specs/041-skills-layer.md). R7 — the per-trail budget and its cap — is the honey reserve that already exists; the observability spec is what makes its spending readable.

### What the platform guarantees, and what it does not

The platform guarantees **comparability**: a fork and the trail it came from carry the same task specification and the same base commit, and differ only by the recorded delta — which is a commit, so the difference is inspectable as a git range. It also guarantees **isolation**: a fork runs on its own plane, with its own bus, home and checkout, and never touches the live colony.

The platform does **not** decide whether the delta was an improvement. That judgement needs a quality criterion — the same accepted result, a cheaper median, a smaller rework count — and a criterion is a methodology's business. This is the whole contract in one sentence: *the platform makes the comparison fair; the methodology decides what better means.*

## The loop

```
     a trail ends
          │
          ▼
    R1/R2  the runtime records what happened and says the trail is finished
          │
          ▼
   methodology decides this trail is worth studying
          │
          ▼
      R3   locate the moment the run diverged
          │
          ▼
      R4   propose a change to .paseka/  (a proposal, not a commit)
          │
          ▼
    human approves the change, or the methodology proposes it as a hypothesis
          │
          ▼
      R5/R6  fork: same task, same base, changed .paseka/, isolated bus
          │
          ▼
      R7   the fork spends its own budget and stops
          │
          ▼
   methodology compares the fork with its source and decides
          │
          ├──  better and confirmed ──► promote (hypothesis → rule)
          └──  worse, or unclear  ──► keep the base, record what did not work
```

Three properties keep this from becoming a self-modifying mess:

- **The base is stable.** Every experiment starts from the committed state; nothing edits the colony's branch as a side effect. A change reaches the colony only as a proposal a human approves.
- **One change at a time.** A candidate that changes several things at once is recorded as *confounded* — no attribution is claimed, and no lesson is written. An invented attribution poisons the library for every later task.
- **Refusal is a normal outcome.** Most candidates are worse, or unclear, and the correct result of an experiment is often "keep what we had". The store must be able to hold that answer without it looking like a failure.

## What lands where

| Spec | Status | What it contributes here |
| --- | --- | --- |
| [043 — Trail observability](../specs/043-trail-observability.md) | Draft | R1–R3: the runtime records the truth about a trail, and what its budget was spent on |
| [042 — Trail fork](../specs/042-trail-fork.md) | Implemented | R5, R6: a new pinned trail from the same base with a recorded configuration delta |
| [041 — Skills layer](../specs/041-skills-layer.md) | Draft | R8: the store, the per-role gate, deterministic delivery and attribution |
| [034 — Swarm mode](../specs/034-swarm-mode.md) | Draft | Fan-out over variants, a judge that cannot see the harness, per-variant stats |
| Nuc pack (not a spec) | — | the methodology itself: the role, its prompts, its skills, its thresholds |

The order is 043 → 041 → 042 → 034 → the pack. The first two are useful without any methodology at all: an operator who can read a trail's metrics and manage a role's skills gets value from them whether or not any colony ever runs the self-improvement loop.

## What this note does not decide

- **Whether the methodology ships.** The split above says where it would live, not that it will be built. A colony may well be better off with the observability spec and nothing else.
- **The quality criterion.** "Better" is the methodology's, and the criteria differ by domain. The platform offers metrics; it does not rank them.
- **How many runs an experiment needs.** Noise discipline is a methodology decision informed by how noisy the local models are.
- **A marketplace.** A registry of Nuc packs, how a pack is signed, and how a pack is pinned are a separate spec. Nothing here assumes one exists.
- **Automatic promotion.** The platform does not approve a change by itself, and does not disable a procedure by itself. Both stay human acts, as [spec 041](../specs/041-skills-layer.md) already decides.

## Where to go next

| Goal | Doc |
| --- | --- |
| Vocabulary | [Glossary](glossary.md) |
| Principles of the runtime | [Principles](principles.md) |
| The skills store and the per-role gate | [Spec 041](../specs/041-skills-layer.md) |
| Forking a trail | [Spec 042](../specs/042-trail-fork.md) |
| What the runtime records about a trail | [Spec 043](../specs/043-trail-observability.md) |
| Fan-out and comparison | [Spec 034](../specs/034-swarm-mode.md) |
| Portable bee packs | [Nuc packs](../guide/nuc.md) |
