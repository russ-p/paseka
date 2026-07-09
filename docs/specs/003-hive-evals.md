# Spec 003: Hive Evals

## Purpose

Define a repeatable evaluation system for Paseka's choreographed hive: side test colonies, seeded initial state, fault injection, and scoring of builder ↔ guard correction loops.

This spec captures the shared design only. Implementation must not start until explicitly confirmed.

## Goals

- Provide a **side eval colony** (separate git repo) with a curated set of tiny, oracle-checkable tasks.
- Make each case **repeatable** from a known seed commit, not from a drifting long-lived branch.
- Support **fault injection** so evals can force `builder mistake → guard notice → builder fix` without relying on LLM luck.
- Score runs on **observable hive outcomes** (event chain, rework cycles, worktree tests), not on free-form agent prose.
- Layer evals into three tiers: in-process choreography, bus integration, and live adapter runs.
- Reuse existing primitives: `traceId`, worktrees, task ledger, bee routing, `paseka purge`, `paseka signal`, `paseka replay`, run IPC under `.paseka/runs/`.

## Non-Goals

- Do not turn `paseka replay` into a re-execution engine in this spec (today it is inspect-only).
- Do not implement `energyToken` / `confidence` budgets here; loop protection remains `maxReworkCycles` until those land.
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
| Routing | [008-bee-routing.md](../008-bee-routing.md) | `task.ready` → builder → `code.proposal` → guard → `verification.*` → builder |
| Rework cap | `maxReworkCycles = 3` in `internal/runtime/review_gate.go` | Stuck loops escalate to `waiting_review` |
| Fixed trace | CLI `--trace` | Stable case identity across resets |
| Purge | `paseka purge --runs --worktrees --state` | Ephemeral cleanup between runs |
| Signal inject | `paseka signal` | Bus-level fault injection |
| Replay | `paseka replay <traceId>` | Read-only event chain inspection |
| Test harness | `recordingAdapter`, `NewTestReactor`, `MemoryLedger` | Tier A in-process evals |
| Builder intent | `_partials/builder-intent-test-fix.md` | Prefer `intent: test-fix` for oracle tasks |

Builder / guard loop (already shipped):

```text
SIGNAL/task.ready
  → builder (MUTATION/code.proposal from diff)
  → guard (VERIFICATION/success|failed)
  → on failed: builder direct dispatch (fix-up)
  → after 3 failed cycles: task → waiting_review
```

Gaps today:

- No `testdata/` / `fixtures/` / golden eval suite.
- No seeded `agentId` control.
- No JetStream snapshot/restore helper for eval namespaces.
- No end-to-end builder→guard→builder integration test with scripted adapters.
- `energyToken` / `confidence` are product vision only.

## Decisions

### Eval colony is a sibling repo

Keep evals **out of** the Paseka platform repo.

Recommended layout:

```text
paseka-eval-colony/                 # separate git repository
├── .paseka/
│   ├── colony.yaml
│   ├── bees/                       # builder, guard, receiver (eval-tuned)
│   └── prompts/                    # may be thinner than production prompts
├── cases/
│   ├── 01-add-function/
│   │   ├── case.yaml
│   │   ├── seed/                   # initial tree checked into the case
│   │   ├── broken/                 # optional intentional bad patch/diff
│   │   └── expect/                 # oracle tests or expected files
│   └── 02-fix-off-by-one/
│       ├── case.yaml
│       ├── seed/
│       └── expect/
├── runner/                         # optional Go/CLI harness (later)
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
   - `paseka purge --runs --worktrees --state --yes`
   - `git worktree prune` and delete leftover `paseka/<trace>` branches if present
   - reset colony working tree to `seedSha`
   - use a dedicated NATS subject prefix / JetStream namespace for eval, or wipe eval consumers/KV between runs

Repeatable inputs:

| Input | How |
| ----- | --- |
| Code baseline | `seedSha` |
| Trace identity | fixed `trace` in case YAML |
| Colony config | committed `.paseka/` in eval repo |
| Bus state | isolated prefix or wipe |
| Apiary state | purge + optional `XDG_CONFIG_HOME` isolation |

Non-deterministic (acceptable at Tier C only):

- LLM token output
- auto-generated `agentId` (unless a future seed flag lands)

### Three eval tiers

| Tier | Name | LLM | Bus | What it proves |
| ---- | ---- | --- | --- | -------------- |
| **A** | Choreography | No (mock / recording adapters) | Optional in-memory | Routing, ledger transitions, rework cap, direct dispatch loop |
| **B** | Bus integration | No (scripted bees or `paseka signal`) | Real NATS | End-to-end subjects, replay chain, purge/reset |
| **C** | Live hive | Yes | Real NATS | Prompt + adapter quality against oracles; report pass@k |

Tier A lives primarily in the Paseka repo as Go tests (extend `internal/runtime` harness).
Tiers B/C live primarily in the eval colony + a thin runner.

### Fault injection modes

Cases declare how the first mistake is introduced:

| Mode | Mechanism | Use |
| ---- | --------- | --- |
| `scripted` | Fake/script adapter (or fixture bee) applies `broken/` patch as first builder result | Deterministic loop tests |
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
- three failures → `waiting_review` escalation
- human reject path (`INSIGHT/human.feedback`) when review policy requires it

### Oracle and scoring

Score **outcomes**, not narratives.

Minimum oracle for a case:

- shell command in the worktree (usually tests), e.g. `go test ./...`
- optional expected event chain (type/kind sequence)
- budgets: `max_rework_cycles`, `max_builder_runs`, wall-clock timeout

Suggested score fields:

| Field | Meaning |
| ----- | ------- |
| `tests_passed` | Oracle command exit 0 in worktree |
| `event_chain_ok` | Observed bus/run events match expected subsequence |
| `rework_cycles` | Count of `verification.failed` for the task |
| `escalated` | Task reached `waiting_review` due to rework cap |
| `builder_runs` | Number of builder dispatches |
| `duration` | Wall time |

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
  mode: scripted            # scripted | inject-mutation | live
  broken_diff: broken/bad.patch
oracle:
  command: "go test ./..."
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

### Runner responsibilities (later implementation)

A runner (script or Go command, location TBD) should:

1. Load `case.yaml`.
2. Reset colony to seed + purge ephemeral state.
3. Ensure NATS/eval namespace is clean.
4. Start or attach to `paseka run` when Tier B/C.
5. Apply fault mode (`scripted` / `inject-mutation` / `live`).
6. Create/start the task with fixed `--trace`.
7. Wait for terminal task status or timeout.
8. Run oracle in the trace worktree.
9. Collect `paseka replay`, run IPC, and ledger snapshot.
10. Emit a machine-readable report (JSON) + short human summary.

Tier A may skip the external runner and stay as Go tests that assert the same event/status expectations.

### Eval-tuned bees

The eval colony may ship simplified bees:

- **Script guard** — runs `oracle.command` (or case tests) and emits `verification.success|failed` without an LLM.
- **Script builder** (Tier A/B) — applies fixture patches / writes known files.
- **Live builder/guard** (Tier C) — real adapters; same contracts.

Production Paseka bees remain unchanged unless a platform gap is found.

### Isolation rules

- Dedicated `XDG_CONFIG_HOME` (or documented apiary slug) for CI eval runs.
- Dedicated NATS URL / subject prefix for eval when sharing a machine with a real colony.
- Never reuse production JetStream KV keys for eval traces without a wipe step.
- Eval colony `.paseka/runs/` and `worktrees/` stay gitignored (normal colony layout).

## Phased delivery

### Phase 0 — Spec + skeleton (this doc)

- Agree on tiers, reset model, case YAML, scoring.
- Optionally create empty `paseka-eval-colony` with one stub case (no automation yet).

### Phase 1 — Tier A in Paseka

- Full in-process test: bad builder diff → guard `verification.failed` → builder re-dispatch → success.
- Test rework escalation after 3 failures → `waiting_review`.
- Reuse `recordingAdapter` / `NewTestReactor` / `MemoryLedger`.

### Phase 2 — Eval colony + Tier B

- Create sibling repo with 1–3 tiny cases (`seed/`, `expect/`, `case.yaml`).
- Scripted guard (and optional scripted builder).
- Reset script: purge + seed checkout + fixed trace.
- Assert event chain via `paseka replay` / run `events.ndjson`.

### Phase 3 — Tier C live suite

- Same cases with real adapters.
- Report `pass@k`, median cycles, failures.
- Keep Tier A/B as merge gates; Tier C as scheduled/manual quality signal.

### Phase 4 — Platform affordances (only if needed)

- Optional `--agent-id` / seed for deterministic run dirs.
- Eval-oriented purge/namespace helpers.
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
# from eval colony root, with NATS up
paseka purge --runs --worktrees --state --yes
# runner or manual:
paseka run &
paseka task create --trace eval-01-add-function --title "..." --body "..." --bee builder --intent test-fix --autorun
paseka replay eval-01-add-function
# oracle inside worktree
```

Success criteria for the design:

- A case can be reset and re-run to the same seed without manual branch archaeology.
- A scripted fault produces a visible guard→builder correction loop.
- Live evals can fail the oracle without failing platform CI.

## Open questions

1. **Runner home** — Go subcommand in Paseka (`paseka eval …`) vs standalone tool in the eval colony?
2. **Seed materialization** — copy `seed/` into a throwaway git repo each run vs tags on a single eval-colony history?
3. **Script bees** — **resolved:** first-class `adapter: script` in platform; eval colonies use script bees with `paseka event emit` for domain events.
4. **JetStream wipe** — document manual wipe, or add `paseka purge --bus` for eval namespaces?
5. **Case language** — keep YAML only, or allow Markdown task bodies with YAML front matter?
6. **Multi-sector cases** — defer until sectors are common in real colonies?

## References

- [003-architecture.md](../003-architecture.md) — colony layout, worktrees, runs IPC
- [005-task-ledger.md](../005-task-ledger.md) — trace / task / agent hierarchy
- [007-cli.md](../007-cli.md) — `run`, `task`, `signal`, `replay`, `purge`
- [008-bee-routing.md](../008-bee-routing.md) — builder/guard subscriptions
- [001-brief.md](../001-brief.md) — event replay and energyToken vision
