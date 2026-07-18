# Spec 003: Hive Evals

## Status

**In progress (Phase 2).** Design is locked. Platform Tier A coverage exists via `internal/runtime` harness tests (routing, energy, review gates). Sibling eval colony [`paseka-eval-colony`](https://github.com/russ-p/paseka-eval-colony) has script bees, reset/run-case runner, and at least two cases (`01-add-function`, `02-energy-exhausted`). Phase 3 (live LLM Tier C) and Phase 4 platform affordances are not started. Gotchas from standing up Phase 2: [backlog](../plans/backlog.md) § Eval colony gotchas.

Resolved since the original draft:

- First-class `adapter: script` in the platform (eval bees use it + `paseka event emit`).
- `paseka purge --bus --trace <traceId>` for per-trace JetStream wipe (stop `paseka run` first).

## Purpose

Define a repeatable evaluation system for Paseka's choreographed hive: side test colonies, seeded initial state, fault injection, and scoring of builder ↔ guard correction loops.

This document is the shared design record; further phases should follow it unless decisions below are revised.

## Goals

- Provide a **side eval colony** (separate git repo) with a curated set of tiny, oracle-checkable tasks.
- Make each case **repeatable** from a known seed commit, not from a drifting long-lived branch.
- Support **fault injection** so evals can force `builder mistake → guard notice → builder fix` without relying on LLM luck.
- Score runs on **observable hive outcomes** (event chain, rework cycles, worktree tests), not on free-form agent prose.
- Layer evals into three tiers: in-process choreography, bus integration, and live adapter runs.
- Reuse existing primitives: `traceId`, worktrees, task ledger, bee routing, `paseka purge`, `paseka signal`, `paseka replay`, run IPC under `.paseka/runs/`.

## Non-Goals

- Do not turn `paseka replay` into a re-execution engine in this spec (today it is inspect-only).
- Do not implement `confidence` budgets here; loop protection uses per-trace `energyToken` (honey reserve).
- Do not require a dedicated eval database or Queen Console UI for the first cut.
- Do not make live LLM evals the only gate; flaky model runs must not block choreography correctness.
- Do not evaluate prompt quality in isolation without a hive outcome oracle.
- Do not commit secrets, adapter credentials, or machine-local apiary state into the eval colony.

## Current System Context

Relevant existing pieces:

| Primitive | Location / behavior | Eval use |
| --------- | ------------------- | -------- |
| Colony + bees | `.paseka/` in a git repo | Side eval colony is a normal colony |
| Worktrees | `.paseka/worktrees/<traceId>/`, branch `paseka/<traceId>`, `BaseSHA` at create | Isolated mutation surface per case run |
| Runs IPC | `.paseka/runs/<traceId>/<agentId>/` | Audit trail: prompts, events, status |
| Task ledger | JetStream KV + `paseka task *` | Case work queue |
| Routing | [bee routing](../reference/bee-routing.md) | `task.ready` → builder → `code.proposal` → guard → `verification.*` → builder |
| Honey reserve | `defaults.energy_budget` in `colony.yaml`; `TraceSnapshot.energyRemaining` in JetStream KV | Each dispatch consumes 1 token; exhausted traces block until `paseka energy add` |
| Fixed trace | CLI `--trace` | Stable case identity across resets |
| Purge | `paseka purge --runs --worktrees --state --bus --trace <traceId>` | Ephemeral cleanup between runs (bus wipe requires stopped reactor) |
| Signal inject | `paseka signal` | Bus-level fault injection |
| Replay | `paseka replay <traceId>` | Read-only event chain inspection |
| Script adapter | `adapter: script` + `command` | Deterministic eval bees without an LLM |
| Test harness | `recordingAdapter`, `NewTestReactor`, `MemoryLedger` | Tier A in-process evals |
| Builder intent | `_partials/builder-intent-test-fix.md` | Prefer `intent: test-fix` for oracle tasks |

Builder / guard loop (shipped):

```text
SIGNAL/task.ready
  → builder (MUTATION/code.proposal from diff)
  → guard (VERIFICATION/success|failed)
  → on failed: builder direct dispatch (fix-up)
  → when honey reserve is empty: task → blocked
```

Gaps remaining:

- No golden `testdata/` suite inside the **platform** repo beyond unit/integration tests (Tier B lives in the sibling eval colony).
- No seeded `agentId` control.
- No JetStream snapshot/restore helper beyond per-trace `purge --bus`.
- No automated event-chain scorer in the runner (oracle + human `paseka replay` inspection today).
- Tier C live LLM suite not started.
- `confidence` filtering is not implemented (out of scope; see backlog).

## Decisions

### Eval colony is a sibling repo

Keep evals **out of** the Paseka platform repo.

Recommended layout (matches current `paseka-eval-colony`):

```text
paseka-eval-colony/                 # separate git repository
├── .paseka/
│   ├── colony.yaml
│   ├── bees/                       # builder, guard, receiver (eval-tuned script bees)
│   └── prompts/                    # may be thinner than production prompts
├── cases/
│   ├── 01-add-function/
│   │   ├── case.yaml
│   │   ├── seed/                   # initial tree checked into the case
│   │   ├── broken/                 # optional intentional bad patch/diff
│   │   └── expect/                 # oracle tests or expected files
│   └── 02-energy-exhausted/
│       ├── case.yaml
│       ├── seed/
│       └── broken/
├── scripts/                        # script-adapter hooks
├── runner/                         # reset + run-case harness
├── reports/                        # gitignored run reports
└── README.md
```

Rationale:

- Platform tests stay fast and hermetic.
- Eval colony can evolve tasks/prompts without coupling to Paseka releases.
- Real `paseka init` / worktree / purge paths are exercised as users would.

### Repeatability = seed SHA + purge + fixed traceId

Do **not** treat a long-lived eval branch as the source of truth. Branches drift.

Canonical reset model per case run:

1. **Seed** — materialize `cases/<id>/seed/` onto a clean git state and record `seedSha` (tag or commit).
2. **Trace** — use a fixed `--trace` from `case.yaml` (e.g. `eval-01-add-function`).
3. **Ephemeral worktree** — runtime creates `.paseka/worktrees/<traceId>/` on `paseka/<traceId>` from current `HEAD` (`BaseSHA`).
4. **Clean slate** before each run:
   - stop `paseka run` if it is active (avoid reactor/KV races during bus purge)
   - `paseka purge --runs --worktrees --state --bus --trace <case-trace> --yes`
   - `git worktree prune` and delete leftover `paseka/<trace>` branches if present
   - reset colony working tree to `seedSha`

Repeatable inputs:

| Input | How |
| ----- | --- |
| Code baseline | `seedSha` |
| Trace identity | fixed `trace` in case YAML |
| Colony config | committed `.paseka/` in eval repo |
| Bus state | `paseka purge --bus --trace <case-trace>` (NATS required; stop reactor first) |
| Apiary state | purge + optional `XDG_CONFIG_HOME` isolation |

Non-deterministic (acceptable at Tier C only):

- LLM token output
- auto-generated `agentId` (unless a future seed flag lands)

### Three eval tiers

| Tier | Name | LLM | Bus | What it proves |
| ---- | ---- | --- | --- | -------------- |
| **A** | Choreography | No (mock / recording adapters) | Optional in-memory | Routing, ledger transitions, energy gate, direct dispatch loop |
| **B** | Bus integration | No (scripted bees or `paseka signal`) | Real NATS | End-to-end subjects, replay chain, purge/reset |
| **C** | Live hive | Yes | Real NATS | Prompt + adapter quality against oracles; report pass@k |

Tier A lives primarily in the Paseka repo as Go tests (extend `internal/runtime` harness).
Tiers B/C live primarily in the eval colony + runner.

### Fault injection modes

Cases declare how the first mistake is introduced:

| Mode | Mechanism | Use |
| ---- | --------- | --- |
| `scripted` | Fake/script adapter (or fixture bee) applies `broken/` patch as first builder result | Deterministic loop tests |
| `always_broken` | Script builder always applies a bad change (e.g. energy exhaustion cases) | Force rework until honey blocks |
| `inject-mutation` | Runner publishes `MUTATION/code.proposal` via `paseka signal` with a bad diff | Skip builder v1; test guard→builder |
| `live` | Real builder may err naturally; oracle decides pass/fail | Statistical quality evals |

Primary success story to cover in fixtures:

```text
builder produces bad change
  → guard emits verification.failed
  → builder fix-up runs with failure context
  → guard emits verification.success
  → (optional) receiver / review gate
```

Also cover:

- guard success on first proposal (no rework)
- honey exhaustion → `blocked` (energy budget)
- human reject path (`INSIGHT/human.feedback`) when review policy requires it

### Oracle and scoring

Score **outcomes**, not narratives.

Minimum oracle for a case:

- shell command in the worktree (usually tests), e.g. `go test ./pkg/...` (prefer narrow paths; see backlog gotchas)
- optional expected event chain (type/kind sequence)
- budgets: `max_rework_cycles`, `max_builder_runs`, wall-clock timeout
- optional expected terminal task status / summary (e.g. energy cases)

Suggested score fields:

| Field | Meaning |
| ----- | ------- |
| `tests_passed` | Oracle command exit 0 in worktree |
| `event_chain_ok` | Observed bus/run events match expected subsequence |
| `rework_cycles` | Count of `verification.failed` for the task |
| `escalated` | Task reached `waiting_review` due to rework / review policy |
| `builder_runs` | Number of builder dispatches |
| `duration` | Wall time |
| `task_status` | Terminal ledger status when asserted |

Tier C aggregates: `pass@k`, median rework cycles, p95 duration.

Do not require golden LLM transcripts in v1.

### Case file format

`cases/<id>/case.yaml` (initial shape):

```yaml
id: 01-add-function
trace: eval-01-add-function
seed: seed/                 # directory materialized before run; runner records seedSha
task:
  title: "Add sum(a, b)"
  body: |
    Implement sum in the seeded package so expect/ tests pass.
  bee: builder
  intent: test-fix
  review: none              # or required/final when testing HITL gates
fault:
  mode: scripted            # scripted | always_broken | inject-mutation | live
  broken_diff: broken/bad.patch
oracle:
  command: "go test ./pkg/..."
  workdir: "."              # relative to worktree (or sector path later)
  max_rework_cycles: 2
  expect_event_chain:
    - type: MUTATION
      kind: code.proposal
    - type: VERIFICATION
      kind: verification.failed
    - type: MUTATION
      kind: code.proposal
    - type: VERIFICATION
      kind: verification.success
score:
  must_pass_tests: true
  max_builder_runs: 3
timeout: 10m
```

Notes:

- `trace` must be stable and unique per case.
- `seed/` should be tiny; prefer one package and a few files.
- Prefer `intent: test-fix` so builder prompts align with oracle-driven tasks.
- Sectors may be added later; v1 assumes colony-root workspace.
- Seed code must be **committed** before worktree create (worktrees come from `HEAD`).

### Runner responsibilities

A runner (script in the eval colony today: `runner/run-case.sh`) should:

1. Load `case.yaml`.
2. Reset colony to seed + purge ephemeral state (`paseka … -C` to the eval root).
3. Ensure NATS/eval namespace is clean.
4. Start or attach to `paseka run` when Tier B/C.
5. Apply fault mode (`scripted` / `always_broken` / `inject-mutation` / `live`).
6. Create/start the task with fixed `--trace`.
7. Wait for terminal task status or timeout.
8. Run oracle in the trace worktree.
9. Collect `paseka replay`, run IPC, and ledger snapshot.
10. Emit a machine-readable report (JSON) + short human summary.

Tier A may skip the external runner and stay as Go tests that assert the same event/status expectations.

### Eval-tuned bees

The eval colony ships simplified bees:

- **Script guard** — runs `oracle.command` (or case tests) and emits `verification.success|failed` without an LLM.
- **Script builder** (Tier A/B) — applies fixture patches / writes known files.
- **Live builder/guard** (Tier C) — real adapters; same contracts.

Production Paseka bees remain unchanged unless a platform gap is found.

### Isolation rules

- Dedicated `XDG_CONFIG_HOME` (or documented apiary slug) for CI eval runs.
- Dedicated NATS URL / subject prefix for eval when sharing a machine with a real colony.
- Never reuse production JetStream KV keys for eval traces without a wipe step.
- Eval colony `.paseka/runs/` and `worktrees/` stay gitignored (normal colony layout).
- Always pass `-C` to `paseka` from runner scripts so cwd cannot target the wrong repo.

## Phased delivery

### Phase 0 — Spec + skeleton — Done

- Agree on tiers, reset model, case YAML, scoring.
- Sibling `paseka-eval-colony` created.

### Phase 1 — Tier A in Paseka — Partial / ongoing

- Platform harness: `recordingAdapter` / `NewTestReactor` / `MemoryLedger`.
- Covered: routing (including `verification.failed` → builder direct dispatch), energy block/unblock, review gates.
- Still useful to add: a single end-to-end in-process golden for bad builder → guard fail → builder fix → success if not already covered by focused reactor tests.

### Phase 2 — Eval colony + Tier B — In progress

- Sibling repo with cases (`01-add-function`, `02-energy-exhausted`, …).
- Scripted builder/guard/receiver bees.
- Reset + run-case scripts; JSON reports under `reports/`.
- Assert outcomes via oracle and `paseka replay` (automated event-chain scorer still backlog).

### Phase 3 — Tier C live suite — Not started

- Same cases with real adapters.
- Report `pass@k`, median cycles, failures.
- Keep Tier A/B as merge gates; Tier C as scheduled/manual quality signal.

### Phase 4 — Platform affordances (only if needed) — Not started

- Optional `--agent-id` / seed for deterministic run dirs.
- Eval-oriented purge/namespace helpers (partially covered by `paseka purge --bus --trace`).
- True prompt replay against historical JetStream chains (product brief “Event Replay”) — separate spec.

## Verification (when implementing)

Platform (Tier A):

```bash
gofmt -w .
go test ./internal/runtime/ -count=1
go build -o paseka ./cmd/paseka
```

Eval colony (Tier B/C):

```bash
# from eval colony root, with NATS up (stop paseka run before bus purge)
paseka -C "$EVAL_ROOT" purge --runs --worktrees --state --bus --trace eval-01-add-function --yes
./runner/run-case.sh 01-add-function
paseka -C "$EVAL_ROOT" replay eval-01-add-function
```

Success criteria for the design:

- A case can be reset and re-run to the same seed without manual branch archaeology.
- A scripted fault produces a visible guard→builder correction loop.
- Live evals can fail the oracle without failing platform CI.

## Open questions

1. **Runner home** — today: standalone scripts in the eval colony. Promote to `paseka eval …` later?
2. **Seed materialization** — copy `seed/` into colony root + commit each run (current runner) vs tags on a single eval-colony history?
3. **Script bees** — **resolved:** first-class `adapter: script` in platform; eval colonies use script bees with `paseka event emit` for domain events.
4. **JetStream wipe** — **resolved:** `paseka purge --bus --trace <traceId>` removes task-ledger KV, stream events, and object-store artifacts for one trace without restarting NATS. Stop `paseka run` before bus purge. See [CLI](../guide/cli.md) § `paseka purge`.
5. **Case language** — keep YAML only, or allow Markdown task bodies with YAML front matter?
6. **Multi-sector cases** — defer until sectors are common in real colonies?
7. **Event-chain scorer** — automate `expect_event_chain` against `paseka replay` (backlog follow-up).

## References

- [architecture overview](../architecture/overview.md) — colony layout, worktrees, runs IPC, script adapter
- [task ledger](../reference/task-ledger.md) — trace / task / agent hierarchy
- [CLI](../guide/cli.md) — `run`, `task`, `signal`, `replay`, `purge`
- [bee routing](../reference/bee-routing.md) — builder/guard subscriptions
- [bee config](../guide/bee-config.md) — `adapter: script` and bee schema
- [Brief](../idea/brief.md) — event replay and energyToken vision
- [backlog](../plans/backlog.md) — eval colony gotchas and harness follow-ups
