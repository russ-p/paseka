# Spec 031: Task Scheduler

## Status

**Draft**
Initial design for time-slot based task scheduling integrated into the reactor.

## Problem Statement

Currently, tasks are either executed immediately when created with `--autorun`, or manually started later via `paseka task start`. There is no mechanism to defer execution to specific time windows (e.g., nightly, weekends) when API demand is lower, costs may be cheaper, or human operators are unavailable for review.

Beekeepers need a way to schedule tasks for off-peak hours without staying awake or remembering to run commands manually. The scheduler should integrate naturally with the existing choreography — tasks remain in the ledger, energy budgets are respected, and operators can override or reschedule at any time via Console or CLI.

## Solution

Add a colony-level scheduler configuration with named time slots (cron expressions or daily windows). Tasks reference a slot by name in `task.plan`; if the slot exists and is valid, the task enters `deferred` status. The reactor runs a background scheduler goroutine that wakes periodically, checks which slots are active, and publishes `task.ready` for eligible deferred tasks. Operators can override via Console/CLI: run now, change slot, or remove slot (revert to immediate).

## User Stories

1. As a Beekeeper, I want to define named time slots in the colony manifest (e.g., `nightly` at 2 AM, `weekend` at 3 AM Sat/Sun), so that tasks can be scheduled declaratively.

2. As a Beekeeper, I want to use both cron expressions (`0 2 * * *`) and simple daily windows (`22:00-06:00`) for slot definitions, so that I can choose the most natural notation.

3. As a Beekeeper, I want to set a colony-level timezone (default UTC), so that slots run at the intended local time regardless of server location.

4. As a Beekeeper, I want to create a task with `--slot nightly` (or `runSlot` in API), so that the task enters `deferred` status and waits for the next `nightly` window.

5. As a Beekeeper, I want the task ledger to show `deferred` tasks separately from `planned`, so that I can see which tasks are waiting for a time slot.

6. As a Beekeeper, I want the reactor to automatically publish `task.ready` for deferred tasks when their slot becomes active, so that execution happens without manual intervention.

7. As a Beekeeper, I want the scheduler to respect the energy/honey budget — if energy is exhausted, deferred tasks wait for the next slot, so that I don't overspend.

8. As a Beekeeper, I want the scheduler to respect task dependencies (`dependsOn`) — a deferred task waits for its dependencies to complete before becoming eligible, so that ordering is preserved.

9. As a Beekeeper, I want multiple slots to be able to overlap (e.g., different providers/models for different bees), so that concurrent windows don't conflict.

10. As a Beekeeper, I want FIFO ordering within an active slot — if multiple deferred tasks are eligible, they are started in creation order, so that behavior is predictable.

11. As a Beekeeper, I want tasks that miss their slot (reactor down) to run on the next reactor tick after startup, so that no scheduled work is silently dropped.

11. As a Beekeeper, I want to run a deferred task immediately via `paseka task run-now --trace <id> --task <id>` or Console "Run now" button, so that I can override the schedule for urgent work.

12. As a Beekeeper, I want to change a task's slot via `paseka task reschedule --trace <id> --task <id> --slot weekend` or Console dropdown, so that I can adjust timing without recreating the task.

13. As a Beekeeper, I want to remove a task's slot (`--slot ""`) to revert to immediate execution, so that I can un-defer a task.

14. As a Beekeeper, I want Console to show the current slot, next run time, and allow inline rescheduling, so that I can manage schedules visually.

15. As a Beekeeper, I want the scheduler to be enabled/disabled via `scheduler.enabled` in colony manifest, so that I can turn it off for testing or colonies that don't need it.

16. As a Beekeeper, I want `paseka task list` to show slot name and next run estimate for deferred tasks, so that I can inspect the schedule from CLI.

## Implementation Decisions

### Colony Manifest Extension

Add `scheduler` section to `.paseka/colony.yaml`:
```yaml
scheduler:
  enabled: true
  timezone: "Europe/Berlin"     # IANA timezone, default UTC
  slots:
    - name: "nightly"
      cron: "0 2 * * *"
    - name: "weekend"
      cron: "0 3 * * 0,6"
    - name: "lunch"
      window: "12:00-13:00"
```

### Task Spec Extension

Add `RunSlot string` to `protocol.TaskSpec` (INSIGHT/task.plan payload). Validation at create time: slot name must exist in colony manifest if provided.

### Task Status Flow

```
planned ──(has valid runSlot)──▶ deferred ──(slot active + eligible)──▶ ready ──▶ running
     ▲                              │
     │                              ▼
     └───────── override ───────▶ planned (slot removed)
```

- `deferred` is a new `protocol.TaskStatus` value
- Ledger applies `deferred` on `task.plan` when `runSlot` is valid
- Slot removal via override publishes `task.ready` immediately (like manual start)

### Scheduler in Reactor

- Background goroutine started in `Reactor.Run()`
- Wakes every 30 seconds (configurable), aligns to minute boundaries
- Loads colony manifest once at startup, re-reads on config change signal (future)
- For each active slot at current time:
  - Query ledger for traces with `deferred` tasks matching slot
  - Filter by `taskledger.CanStart()` (dependencies, energy, bee subscribed)
  - Sort eligible tasks by creation time (FIFO)
  - Publish `task.ready` for each
- Missed windows: on reactor startup, run one catch-up tick for slots that were active in the last interval

### Energy Budget

Scheduler does NOT bypass energy checks. `dispatchReady` already calls `gateDispatchEnergy`. If energy exhausted, task stays `deferred` and retries next slot.

### Overlap Handling

Multiple slots can be active simultaneously. Scheduler collects union of active slots, processes each deferred task once per tick (dedup by task key). No limit on concurrent tasks per slot — energy and bee capacity are the natural limits.

### Timezone

Colony manifest `scheduler.timezone` (IANA name). Default UTC. All cron/window evaluation uses this timezone. Stored in ledger metadata for Console display.

### Console/CLI Override

New CLI commands:
- `paseka task run-now --trace <id> --task <id>` — publishes `task.ready` immediately, clears `runSlot`
- `paseka task reschedule --trace <id> --task <id> --slot <name|"">` — updates `runSlot`, re-evaluates status

Console API:
- `POST /api/traces/:traceId/tasks/:taskId/run-now`
- `POST /api/traces/:traceId/tasks/:taskId/reschedule { "slot": "weekend" }`

### Ledger Operations

New ledger methods:
- `ListTracesWithStatus(status TaskStatus) ([]string, error)` — efficient index scan
- `UpdateTaskRunSlot(traceID, taskID, newSlot string) error` — atomic slot change + status transition

### Event Contracts

No new event kinds. Uses existing:
- `SIGNAL/task.ready` — published by scheduler (agentId: "scheduler")
- `INSIGHT/task.plan` — carries `runSlot` on create
- `VERIFICATION/task.completed` — normal completion

### FIFO Ordering

Tasks sorted by `CreatedAt` (from ledger snapshot) when multiple eligible in same tick.

## Testing Decisions

- Unit tests for slot evaluation (cron, window, timezone, overlap)
- Unit tests for scheduler tick logic with mocked ledger/time
- Integration test: create task with slot, advance fake time, verify `task.ready` published
- Integration test: energy exhaustion defers to next slot
- Integration test: dependency blocking within slot
- Integration test: reactor restart catches up missed slot
- CLI tests: `run-now`, `reschedule` commands
- Console API tests: override endpoints

Prior art: `internal/runtime/reactor_test.go`, `internal/tasks/ops_test.go`, `internal/taskledger/*_test.go`

## Out of Scope

- Dynamic slot reconfiguration without reactor restart (future: watch colony.yaml)
- Per-task cron overrides (use colony slots only)
- Priority queues beyond FIFO (energy + bee capacity sufficient for MVP)
- Distributed scheduler coordination (single reactor per colony)
- Notifications/alerts when task runs (existing artifact/artifact.written covers this)
- Historical schedule analytics

## Further Notes

- Slot names are colony-scoped; no cross-colony references
- `runSlot` is optional; tasks without it behave as today (planned → ready on manual start or --autorun)
- Scheduler agentId "scheduler" appears in task run projections for auditability
- Console should poll next-run estimates from API (computed from slot schedule + current time)