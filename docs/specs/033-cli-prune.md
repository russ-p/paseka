# Spec 033: Age-Based Prune

## Status

**(Implemented)**
`paseka prune` removes `.paseka/worktrees/` and `.paseka/runs/` trace directories older than a retention period (default 14 days), with optional correlatable NATS cleanup.

## Problem Statement

`paseka purge` is all-or-nothing per artifact class: `--runs` deletes every trace's run directory and `--worktrees` deletes every isolated worktree, with no way to keep the traces a Beekeeper worked on recently. On a long-lived colony that means the operator must either wipe everything and lose recent context, or hand-delete directories one by one. Trailing bus state (task-ledger KV entries, stream events, artifacts) for trails that no longer exist on disk also accumulates with no age-aware cleanup.

## Solution

A new `paseka prune` command removes stale artifacts by age instead of removing all of them. It scans `.paseka/worktrees/` and `.paseka/runs/` for trace directories whose last activity predates a configurable retention period (`--older-than`, default `14d`), shows a plan, and asks for confirmation unless `--yes` is passed. When NATS is configured and `--bus` is requested, prune also removes task-ledger KV, stream events, and artifacts for correlatable traces, and uses ledger activity to protect traces that are still active even if their files have not changed recently. Filesystem flags mirror `paseka purge` (`--runs`, `--worktrees`, `--all`, `--yes`, `--path`), so the command is a drop-in age-aware sibling rather than a replacement.

## User Stories

1. As a Beekeeper, I want to remove worktrees and run data older than a retention period, so that the colony does not grow without bound while recent work stays available.
2. As a Beekeeper, I want a sensible default retention of 14 days, so that I can run `paseka prune` without remembering a flag.
3. As a Beekeeper, I want to set the retention as `14d`, `2w`, or a Go duration such as `336h`, so that I can express the window in the unit I think in.
4. As a Beekeeper, I want to prune only run data or only worktrees when I choose, so that I can keep worktrees around while reclaiming disk from run logs.
5. As a Beekeeper, I want `--all` to mean runs plus worktrees, so that the flag matches `paseka purge --all`.
6. As a Beekeeper, I want to see a plan of exactly which traces will be removed and when each was last used before anything is deleted, so that I can catch surprises.
7. As a Beekeeper, I want `--yes` to skip the confirmation prompt for scripts and cron jobs, so that routine cleanup can be automated safely.
8. As a Beekeeper, I want pruned worktrees unregistered from machine-local state, so that the worktree registry does not accumulate dangling rows.
9. As a Beekeeper, I want a worktree with a recent run for the same trace to be kept, so that an active trail is never deleted just because no worktree file changed recently.
10. As a Beekeeper with NATS configured, I want `--bus` to also remove stale task-ledger KV entries, stream events, and artifacts, so that bus state is cleaned alongside filesystem state.
11. As a Beekeeper with NATS configured, I want ledger task activity to override stale file mtimes, so that a trace whose filesystem has been idle but whose tasks were touched recently is protected from pruning.
12. As a Beekeeper with NATS configured, I want stale ledger-only traces (no filesystem directory left) to be cleaned up, so that orphan bus entries do not linger forever.
13. As a Beekeeper, I want ledger entries with no task timestamps to be skipped rather than guessed at, so that uncorrelatable traces are never pruned by age.
14. As a Beekeeper, I want `--bus` to fail closed when NATS is not configured, so that a request to clean bus state never silently does nothing.
15. As a Beekeeper, I want a worktree with uncommitted changes to be kept even when idle, so that prune never discards work in progress.
16. As a Beekeeper, I want `paseka prune` to print "Nothing to prune." when no trace is old enough, so that a scheduled prune is quiet on a fresh colony.
17. As a Beekeeper, I want prune to report what it removed, including per-trace bus counts, so that I can audit a cleanup run.
18. As an operator, I want prune to be safe to run on a Standing Trail: only the standing trace's age matters, and its comb is untouched while it is active.
19. As a maintainer, I want the retention parser and age scanning covered by focused tests, so that the destructive path stays predictable.

## Implementation Decisions

### 1. Retention period

- `--older-than` accepts an integer or decimal with a `d` (days) or `w` (weeks) suffix, or any Go duration string. The default is `14d`.
- Zero, negative, or unparseable periods are rejected before any scan.
- A cutoff is computed once as `now - retention` and used for the whole plan so the confirmed view matches the deletion set.

### 2. Target selection

- `--runs` and `--worktrees` select artifact classes; `--all` selects both. When neither `--runs` nor `--worktrees` is given and `--bus` is not the only explicit target, prune defaults to runs plus worktrees.
- `--bus` is explicit and never implied by `--all`, matching `paseka purge`.
- `--yes`/`-y` and `--path`/`-C` mirror `paseka purge`.

### 3. Age determination

- Run-directory activity is the newest file or directory modification time under `.paseka/runs/<traceId>/` (run directories are small).
- Worktree activity combines the worktree directory, its `.git` pointer file, and the run-directory activity for the same trace. Full checkouts are not walked so prune stays cheap on large repositories.
- A worktree with uncommitted or untracked changes is marked protected and is never auto-removed, regardless of age.
- When `--bus` is set, task-ledger activity (the newest `updatedAt` across the trace's tasks) is layered on top; the newest of filesystem and ledger activity wins.

### 4. Bus correlation

- `--bus` lists every trace in the task-ledger KV bucket with its newest task update time.
- Ledger activity is used both to protect active filesystem traces and to list ledger-only traces whose activity predates the cutoff.
- A ledger entry with no task timestamps is not correlatable and is skipped.
- Bus cleanup reuses the existing per-trace purge (task-ledger key, matching domain events, `<traceId>-*.diff` artifacts) and aggregates results.

### 5. Safety

- The plan is computed once before confirmation and executed from that plan, so the displayed view and the deletion set cannot drift.
- Every trace is considered independently; a recent run protects its worktree but does not prevent pruning of run directories for other traces.
- Pruned worktrees are unregistered from the machine-local state registry.

## Testing Decisions

Good tests assert external behavior: which trace directories survive, which are removed, and which bus traces are selected, not internal map shapes.

- Retention parsing is table-driven over day, week, and Go-duration inputs plus rejection cases.
- Age scanning tests create run and worktree fixtures, backdate them with `os.Chtimes`, and assert the plan and execute results, mirroring `internal/purge/fs_test.go`.
- Bus target selection is tested directly for the ledger-override, ledger-only, and uncorrelatable cases.
- An integration-tagged test applies old and recent `task.plan` events to a real NATS ledger and asserts the old trace is purged while the recent one survives, following `internal/purge/purge_bus_integration_test.go`.
- CLI tests exercise default target selection, `--yes`, and bad retention parsing, following `cmd/paseka/status_test.go`.

## Out of Scope

- Replacing or deprecating `paseka purge`; both commands remain available.
- Age-based cleanup of `.paseka/cache/`, machine-local sessions, invites, or pull-request registry rows.
- Automatic/scheduled pruning (a cron or reactor-driven trigger).
- A Queen Console surface for prune; this is a CLI-only operator tool.
- Age-based pruning of the trace comb independently of the run directory.

## Further Notes

Related: [003-hive-evals](./003-hive-evals.md) trace resets use `paseka purge`; prune complements it for routine hygiene. Durable behavior is documented in the [CLI guide](../guide/cli.md) (`paseka prune`) next to `paseka purge`.
