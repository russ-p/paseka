# Backlog

## Deferred Ideas

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
