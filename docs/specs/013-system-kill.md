# Spec 013: Hard system.kill

## Status

**Implemented.** Hard per-trace kill via `SIGNAL/system.kill`: ledger gate, in-flight AFK cancel, CLI `paseka kill`.

## Problem Statement

When a trace enters a dispute avalanche or runaway agent loop, honey depletion only blocks **future** dispatches — in-flight adapter processes keep running. Beekeepers need an emergency stop that halts the trace immediately without shutting down the whole hive.

## Solution

Publish `SIGNAL/system.kill` for a `traceId`. The reactor marks the trace killed, cancels non-terminal tasks, stops new dispatches, and cancels in-flight AFK adapter contexts. Operators invoke it via `paseka kill --trace <id>`.

## User Stories

1. As a Beekeeper, I want to kill a trace from the CLI, so that I can stop a bad loop without waiting for honey to drain.
2. As a Beekeeper, I want in-flight AFK agent processes cancelled when I kill a trace, so that Cursor/Pi/Claude children do not keep editing after I say stop.
3. As a Beekeeper, I want killed traces to reject new task/direct dispatches, so that the avalanche cannot resume on the same trace.
4. As a Beekeeper, I want `energy.add` after a kill to top up honey but not redispatch killed tasks, so that kill is a terminal flight decision.
5. As a Beekeeper, I want repeated kill on the same trace to be idempotent, so that double-clicks do not corrupt ledger state.
6. As a Beekeeper, I want non-terminal tasks (`planned`, `ready`, `running`, `waiting_review`, `blocked`) marked `cancelled` with an operator reason, so that Console/CLI show why the trace stopped.
7. As a Beekeeper, I want `completed` and `failed` tasks left unchanged on kill, so that finished work remains visible.
8. As a platform operator, I want `system.kill` on the platform SIGNAL denylist, so that misconfigured bees cannot AFK-dispatch on kill events.
9. As a Beekeeper running `paseka kill` while the reactor is stopped, I want the ledger updated locally, so that offline inspection reflects the kill.
10. As a Beekeeper, I want kill to work when honey remains, so that I do not have to burn the full budget before stopping.

## Implementation Decisions

### Event contract

- Type: `SIGNAL`
- Payload kind: `system.kill`
- Fields: `reason` (optional string)
- Envelope: `traceId` required; `agentId` = publisher (`cli`, later `console` / `telegram`)

### Ledger reducer

- `TraceSnapshot.killed` (bool, sticky)
- On first kill: set `killed=true`; transition non-terminal tasks to `cancelled` with summary from `reason` or default `Trace killed by operator`
- No `Ready` tasks emitted from kill Apply
- Idempotent when already killed

### Reactor

- After ledger Apply on `system.kill`, cancel all registered in-flight dispatch contexts for that `traceId`
- Replace `inflight` / `directInflight` dedupe maps with `map[string]context.CancelFunc`
- Each AFK dispatch uses `context.WithCancel` passed to adapters
- Gate `dispatchReady` / `dispatchDirect` / `unblockEnergyBlockedTasks` when `snap.Killed`
- When dispatch returns `cancelled` and trace is killed, do not downgrade to `failed`

### Operator MVP

- CLI: `paseka kill --trace <id> [--reason …]` — publish + wait KV when reactor running; local Apply when stopped
- Queen Console / Telegram — follow-up; same bus kind

### Task status

- New `TaskStatus`: `cancelled`

## Testing Decisions

- **Ledger:** pure `ApplyEvent` tests for kill semantics, idempotency, terminal task preservation
- **Reactor:** `NewTestReactor` with mock dispatcher that blocks until ctx cancelled; assert kill cancels and task stays `cancelled`
- **CLI/tasks:** publish path mirrors `AddEnergy` wait helper
- Prior art: `internal/runtime/energy_test.go`, `internal/taskledger/energy_test.go`

## Out of Scope

- Per-task or per-agent kill scope
- Interactive `bee chat` / `session stop` via `system.kill`
- Console / Telegram surfaces in MVP
- Resume / unkill API
- New trace from interrupted worktree (backlog follow-up)
- Worktree cleanup on kill

## Further Notes

- Complements honey: honey = soft stop (block next dispatch); kill = hard stop (cancel in-flight + gate)
- See [Backlog](../plans/backlog.md) § New trace from interrupted worktree for continuing good work after kill
