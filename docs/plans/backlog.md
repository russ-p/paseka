# Backlog

Deferred ideas, follow-ups, bugs, and implementation assumptions outside the active change.
Shipped work: [Changelog](changelog.md). Design drafts: [Specs index](specs-index.md).

## Deferred work

### Run artifacts

AFK runs still expose `result.txt` as a familiar artifact from the early bash prototype. Success semantics should move fully to `INSIGHT/run.summary`; the file may remain a human-readable log afterward.

#### Rename `result.txt` to a log-oriented name

- **Kind:** follow-up
- **Source:** planning (run.summary migration)
- **Summary:** After runtime-first `INSIGHT/run.summary` is complete, rename `result.txt` to `summary.md` (or similar) so the path reads as a log, not a success handshake.
- **Why deferred:** Renaming now widens the migration across prompts, runtime context, docs, tests, and external assumptions about run-directory layout. Keeping `result.txt` as a compatibility log path reduces risk while success semantics change.
- **Revisit when:** AFK success no longer depends on reading `result.txt`; runtime enforces or auto-publishes `INSIGHT/run.summary`; prompts and docs no longer treat the file as the run-success handshake.

### Energy and honey

MVP shipped per-trace honey (`defaults.energy_budget`, `energy.add` / `energy.consume`, reactor gating, `paseka energy show|add`). Loop protection is energy depletion → `blocked` (`Honey reserve exhausted`). These items need separate design or evidence before expanding the MVP.

#### `confidence` (Pollen Quality)

- **Kind:** idea
- **Source:** [Brief](../idea/brief.md); planning (energyToken)
- **Summary:** Filter or weight events by confidence level alongside honey.
- **Why deferred:** Needs protocol and UX design (event shapes, CLI/Console) beyond the anti-loop MVP.
- **Revisit when:** Product brief item is specified with event shapes and operator surfaces, or eval scenarios require confidence filtering.

#### `system.kill`

- **Kind:** idea
- **Source:** [Brief](../idea/brief.md); planning (energyToken)
- **Summary:** Bus signal to forcibly stop a trace or agent dispute avalanche. HITL top-up exists (`paseka energy add`); no kill primitive yet.
- **Why deferred:** Needs its own protocol and UX design; orthogonal to shared-budget MVP.
- **Revisit when:** Spec covers event shape and CLI/Console/`gate` surfaces, or operators need an emergency stop beyond energy block.

#### Energy gate on `paseka bee run` / `bee chat`

- **Kind:** follow-up
- **Source:** planning (energyToken)
- **Summary:** One-shot `bee run` and `bee chat` bypass the reactor today; only paths through `paseka run` consume honey. Gate standalone invocations the same way.
- **Why deferred:** Requires adapter-layer changes without a running reactor.
- **Revisit when:** Operators need honey accounting for one-shot/interactive launches, or eval/product rules demand it.

#### Per-bee cost multipliers

- **Kind:** idea
- **Source:** planning (energyToken)
- **Summary:** Charge more than flat `1` per adapter dispatch (per-role or per-intent pricing in bee YAML or routing rules).
- **Why deferred:** Extra configuration surface before evidence that flat cost is too coarse.
- **Revisit when:** Operators report false positives/negatives from flat `1`-token cost or traces that block incorrectly.

#### Honey ↔ LLM token billing

- **Kind:** idea
- **Source:** planning (energyToken)
- **Summary:** Optionally relate honey spend to LLM `usage` on AFK `result.json`. Do not price honey from model tokens without a separate design.
- **Why deferred:** Orthogonal to anti-loop honey; mixing billing models needs an explicit decision.
- **Revisit when:** Product wants cost visibility tied to model usage, with a written design.

#### Interactive session usage

- **Kind:** follow-up
- **Source:** planning (energyToken / SessionAdapter)
- **Summary:** Surface Cursor stream-json `usage` from `bee chat` / SessionAdapter (AFK may already persist optional usage on `result.json`).
- **Why deferred:** Out of energy MVP scope; interactive path differs from AFK file IPC.
- **Revisit when:** Console/CLI need session token usage, or billing/observability work starts.

#### `paseka inspect` usage one-liner

- **Kind:** follow-up
- **Source:** planning (energyToken)
- **Summary:** Optional CLI dump of run/trace usage aggregates (Console/API already cover much of this).
- **Why deferred:** Nice-to-have; not required for honey MVP.
- **Revisit when:** Operators want a terminal one-liner without opening Console.

### Queen Console

API fields for energy and merge-diff exist; SPA polish and per-run proposal preview are still thin.

#### Energy UI polish

- **Kind:** follow-up
- **Source:** planning (energyToken / Queen Console)
- **Summary:** Richer badges, alerts, and Beekeeper actions for `energyBudget`, `energyRemaining`, `lowEnergy` (API fields already exist).
- **Why deferred:** MVP prioritized ledger + reactor gating over SPA chrome.
- **Revisit when:** Operators need at-a-glance honey UX in Console beyond raw API fields.

#### Per-run proposal diff in Reviews

- **Kind:** follow-up
- **Source:** [002-queen-console-mvp](../specs/002-queen-console-mvp.md); planning (reviews)
- **Summary:** Side-by-side preview of per-run `MUTATION/code.proposal.isolated` / `code.proposal.root` for `review: required` tasks. Final merge gate preview (`GET /api/traces/:traceId/merge-diff`) already ships.
- **Why deferred:** Final merge gate was enough for MVP; per-run preview is extra UI surface.
- **Revisit when:** Beekeepers need mid-trace proposal diffs without waiting for the merge gate.

### Code proposal workspaces

Leftovers from [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md).

#### `proposal_paths` allowlist

- **Kind:** follow-up
- **Source:** [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- **Summary:** Restrict which paths may appear in a code proposal.
- **Why deferred:** Not required for dual isolated/root proposal MVP.
- **Revisit when:** Colonies need path policy to limit proposal scope.

#### Untracked files in proposal delta

- **Kind:** follow-up
- **Source:** [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- **Summary:** Include or define behavior for untracked files in proposal deltas.
- **Why deferred:** Deferred from 008 ship to keep delta semantics simple.
- **Revisit when:** Real proposals lose important untracked files, or operators ask for explicit rules.

#### Alias removal for bare `code.proposal`

- **Kind:** follow-up
- **Source:** [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md)
- **Summary:** Timeline and migration to remove the bare `code.proposal` alias (today → isolated).
- **Why deferred:** Alias keeps older colonies working while `.isolated` / `.root` settle.
- **Revisit when:** Docs and colonies have moved to explicit kinds and the alias is a liability.

### Eval harness

Follow-ups for [003-hive-evals](../specs/003-hive-evals.md) and the side colony `paseka-eval-colony`. Gotchas from standing up Phase 2 are under [Assumptions and gotchas](#assumptions-and-gotchas).

#### Task retry with edit

- **Kind:** follow-up
- **Source:** [003-hive-evals](../specs/003-hive-evals.md); planning (task ledger / Console)
- **Summary:** Allow changing bee, intent, body, or sector when retrying a failed task (CLI flags or Console form). Today `paseka task retry` and Console Retry reuse the ledger snapshot as-is.
- **Why deferred:** Snapshot reuse was enough for MVP retry; edit-on-retry needs UX and ledger rules.
- **Revisit when:** Operators or eval cases need corrected retries without creating a new task.

#### Trace reset helper

- **Kind:** follow-up
- **Source:** [003-hive-evals](../specs/003-hive-evals.md); planning (eval harness)
- **Summary:** One command to seed energy and clear ledger for a fixed `--trace` (partially covered by `paseka purge --bus --trace`; a dedicated helper could also re-seed `defaults.energy_budget`).
- **Why deferred:** Purge covers most wipe needs; a friendlier eval helper is polish.
- **Revisit when:** Eval runners repeatedly need seed+wipe in one step.

#### Event-chain scorer in runner

- **Kind:** follow-up
- **Source:** [003-hive-evals](../specs/003-hive-evals.md)
- **Summary:** Assert `case.yaml` `expect_event_chain` against `paseka replay` output (today: oracle + human replay inspection only).
- **Why deferred:** Phase 2 focused on colony wiring and oracle; automated chain scoring is Phase-adjacent polish.
- **Revisit when:** Eval cases rely on event-chain assertions beyond manual replay.

### Releases

#### Windows release builds

- **Kind:** idea
- **Source:** planning (GoReleaser / cross-compile)
- **Summary:** Make `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/paseka` work (Unix-only PTY/HITL today: e.g. `SIGWINCH`, review `Setsid`), then add `windows` to GoReleaser `builds.goos` so release assets include `.exe` archives.
- **Why deferred:** Pipeline already ships linux/darwin from Ubuntu with `CGO_ENABLED=0`; Windows needs build tags/stubs before CI/release changes.
- **Revisit when:** Local/CI Windows cross-build succeeds and release should publish `windows/amd64` (optionally `windows/arm64`).

## Assumptions and gotchas

### Eval colony

Wiring the side eval colony (`paseka-eval-colony`) and `runner/run-case.sh` against real NATS + `paseka run`. See [003-hive-evals](../specs/003-hive-evals.md).

- **Always pass `-C` to `paseka` from runner scripts** — resolving from cwd alone can target the wrong git repo (e.g. parent `paseka` platform) and `purge` the wrong colony. Use `paseka … -C "${EVAL_ROOT}"`.
- **Worktrees are created from `HEAD`, not the working tree** — seed code (`go.mod`, `pkg/`, …) must be **committed** before a trace worktree is created. Do not gitignore materialized seed files at the colony root. Also relevant outside eval; see [008](../specs/008-code-proposal-workspaces.md).
- **Script bees run from the worktree checkout** — `scripts/*.sh` and bee YAML come from git `HEAD`. Uncommitted script changes are invisible inside `.paseka/worktrees/<traceId>/`.
- **`paseka event emit` from script bees needs `-C "$PASEKA_COLONY_ROOT"`** — when cwd is the worktree, emit without `-C` fails colony/home resolution. Guard/receiver scripts must pass colony root explicitly.
- **`paseka event emit` can fail after a successful bus publish** — if audit log append to `.paseka/runs/<traceId>/<agentId>/events.ndjson` fails, emit exits non-zero and the adapter run is marked failed. Normal adapter runs have a run dir; ad-hoc manual emits need a matching run dir.
- **Fixed `trace` + JetStream state accumulates** — reusing case traces leaves ledger KV, depleted honey, and replay history. Use `paseka purge --bus --trace <case-trace>` (stop `paseka run` first); see [CLI](../guide/cli.md) § `paseka purge`.
- **Only one `paseka run` consumer per colony subject prefix** — a second reactor logs `consumer is already bound to a subscription`. Stop the previous runtime before `run-case.sh` starts another.
- **Builder rework is async** — `verification.failed` → builder fix-up via direct dispatch can continue while the task is `waiting_review` or after `completed` (e.g. honey exhausted). Allow time for the guard→builder loop; treat `blocked` as terminal when honey runs out.
- **Oracle scope** — `go test ./...` in the worktree also picks up packages under `cases/…/expect/`. Prefer a narrow path (e.g. `go test ./pkg/...`) in `case.yaml` `oracle.command` and in script-guard bees.
