# Backlog

## Deferred Ideas

### Feature ideation choreography (classify → invite → spec)

Context:
- Raw feature ideas should not jump straight to Scout `task.plan`.
- Drone already has `grilling` / `breakdown` intents; interactive sessions exist but are not bus-driven.
- Design and event shapes are recorded in [specs/005-feature-ideation-flow.md](specs/005-feature-ideation-flow.md).

Backlog items:

- ~~**Phase 0** — Scout `classify` prompt + Drone grilling guidance to write `docs/specs/…` and emit `SIGNAL/spec.ready` (manual `bee chat` path).~~ **Done** — prompts shipped; see [007-cli.md](007-cli.md) soft path.
- ~~**Phase 1** — Platform vs colony SIGNAL boundary: colony ideation kinds (`feature.*`, `spec.ready`) stay out of `internal/protocol`; payload tables remain in the spec + prompts.~~ **Done** — see spec § Decision 8.
- ~~**Phase 2** — Invite persistence, CLI `paseka invite *`, Console accept → session; validate `session.invite` / `beekeeper.ready` at the invite boundary.~~ **Done** — see [007-cli.md](007-cli.md) invite commands.
- ~~**Phase 3** — Config-driven `auto_invites` in `colony.yaml` (default grill rule on `feature.classified` + `route=grill`) while `paseka run` is up.~~ **Done** — reactor loops colony rules; no manual `invite record` for the grill step; empty `auto_invites` disables auto-invite.
- **Phase 4** — Hardening (`spec.ready` completion checks, session energy policy); optional Console “Start breakdown” on `spec.ready`.

Why deferred (Phase 4):
- Completion and energy policy after auto-invite MVP.

Exit criteria for revisiting:
- Beekeeper needs breakdown invite UX or grilling completion enforcement.

### Replace `result.txt` with a clearer log artifact

Context:
- The current AFK adapter flow still uses `result.txt` as a familiar run artifact inherited from the earliest bash-based prototype.
- During the `run.summary` migration, the runtime should stop depending on this file for success semantics.
- The file may still be useful as a human-readable log written by runtime after summary normalization.

Backlog item:
- Consider renaming `result.txt` to `summary.md` or another clearer log-oriented filename after the runtime-first `INSIGHT/run.summary` migration is complete.

Why deferred:
- Renaming the file now would broaden the migration unnecessarily.
- It would require coordinated updates across prompt templates, runtime context, docs, tests, and any external assumptions about run-directory layout.
- Keeping `result.txt` temporarily as a compatibility log path reduces migration risk while the success model is being changed.

Exit criteria for revisiting:
- AFK run success no longer depends on reading `result.txt`.
- Runtime auto-publishes or enforces `INSIGHT/run.summary` consistently.
- Prompt templates and docs no longer describe the file as the run-success handshake.

### energyToken follow-ups (out of MVP scope)

Context:
- MVP shipped per-trace honey reserve (`energyToken`): `defaults.energy_budget` in `colony.yaml`, `energy.add` / `energy.consume` on the task ledger, dispatch gating in the reactor, and `paseka energy show|add`.
- Loop protection moved from `maxReworkCycles` to energy depletion → `blocked` with summary `Honey reserve exhausted`.

Backlog items:

- **`confidence` (Pollen Quality)** — filter or weight events by confidence level; mentioned in [001-brief.md](001-brief.md) alongside `energyToken`.
- **`system.kill`** — bus signal to forcibly stop a trace or agent dispute avalanche; HITL energy injection exists via `paseka energy add`, but no kill primitive.
- **Queen Console UI polish** — API fields (`energyBudget`, `energyRemaining`, `lowEnergy`) exist; richer badges, alerts, and beekeeper actions in the SPA are not done.
- **Per-run proposal diff in Reviews** — final merge gate preview (`GET /api/traces/:traceId/merge-diff`) ships; side-by-side preview of per-run `MUTATION/code.proposal` for `review: required` tasks does not.
- **Energy gate on `paseka bee run` / interactive chat** — one-shot `bee run` and `bee chat` bypass the reactor; only AFK/HITL paths through `paseka run` consume honey today.
- **Per-bee cost multipliers** — every adapter dispatch costs `1`; no per-role or per-intent pricing.

Why deferred:
- MVP focused on the minimum anti-loop loop: shared trace budget, dispatch consume, HITL top-up, and durable ledger state.
- `confidence` and `system.kill` need separate protocol and UX design before implementation.
- Gating standalone bee invocations requires adapter-layer changes without a running reactor.
- Per-bee costs add configuration surface (`bees/*.yaml` or routing rules) before there is evidence the flat cost is too coarse.

Exit criteria for revisiting:
- Operators report false positives/negatives from flat `1`-token dispatch cost or exhausted traces that should not block.
- Product brief items (`confidence`, `system.kill`) are specified with event shapes and CLI/console surfaces.
- Eval harness ([specs/003-hive-evals.md](specs/003-hive-evals.md)) needs scenarios that depend on one of the deferred primitives.

## Eval colony gotchas (Tier B)

Context: wiring the side eval colony (`paseka-eval-colony`) and `runner/run-case.sh` against real NATS + `paseka run`. See [specs/003-hive-evals.md](specs/003-hive-evals.md).

Gotchas observed while standing up Phase 2:

- **Always pass `-C` to `paseka` from runner scripts** — resolving from cwd alone can target the wrong git repo (e.g. parent `paseka` platform repo) and `purge` the wrong colony. Runner helpers must use `paseka … -C "${EVAL_ROOT}"`.
- **Worktrees are created from `HEAD`, not the working tree** — seed code (`go.mod`, `pkg/`, …) must be **committed** before a trace worktree is created. Do not gitignore materialized seed files at the colony root.
- **Script bees run from the worktree checkout** — `scripts/*.sh` and bee YAML come from git `HEAD`. Uncommitted script changes are invisible inside `.paseka/worktrees/<traceId>/`.
- **`paseka event emit` from script bees needs `-C "$PASEKA_COLONY_ROOT"`** — when cwd is the worktree, emit without `-C` fails colony/home resolution (`home config points to … but repo is …/worktrees/<traceId>`). Guard/receiver scripts must pass colony root explicitly.
- **`paseka event emit` can fail the script even after a bus publish** — if audit log append to `.paseka/runs/<traceId>/<agentId>/events.ndjson` fails, emit exits non-zero and the adapter run is marked failed. During a normal adapter run the run dir exists; ad-hoc manual emits need a matching run dir.
- **Fixed `trace` + JetStream state accumulates** — reusing `eval-01-add-function` across runs leaves task-ledger KV entries, depleted honey, and replay history unless wiped. Use `paseka purge --bus --trace <case-trace>` (stop `paseka run` first); see [007-cli.md](007-cli.md) § `paseka purge`.
- **Only one `paseka run` consumer per colony subject prefix** — starting a second reactor logs `consumer is already bound to a subscription`. Stop the previous runtime before `run-case.sh` starts another.
- **`review: none` completes the task after builder success** — runtime publishes `VERIFICATION/task.completed` when the builder adapter succeeds, before guard/receiver direct dispatches finish. Eval scoring should poll the **oracle** (worktree tests) or replay events, not rely solely on task status reaching `completed`.
- **Builder rework is async** — `verification.failed` → builder fix-up uses the direct dispatch path and can continue after the task is already `completed` or `blocked` (e.g. honey exhausted). Allow time for the guard→builder loop; treat `blocked` as a terminal status when honey runs out.
- **Oracle scope** — `go test ./...` in the worktree also picks up packages under `cases/…/expect/`. Prefer a narrow path (e.g. `go test ./pkg/...`) in `case.yaml` `oracle.command` and in script-guard bees.

Backlog follow-ups (eval harness):

- **Platform: optional trace reset helper** — seed energy + clear ledger for a fixed `--trace` in one command (partially covered by `paseka purge --bus --trace`; a dedicated helper could also re-seed `defaults.energy_budget`).
- **Event-chain scorer in runner** — assert `case.yaml` `expect_event_chain` against `paseka replay` output (today: oracle + human replay inspection only).
