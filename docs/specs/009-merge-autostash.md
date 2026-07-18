# Spec 009: Merge Autostash on Approve

## Status

**Implemented.** Shipped 2026-07-18 in `62b8128` (autostash + outcome threading), `8f5de62` (stash-left-on-failure test), and `b3e4351` (preserve merge on pop conflict). Design from flight trail `trace-019f73945163fe35`.

## Purpose

When a human approves an isolated final merge gate, merge the trace worktree into the colony root even if the root working tree is dirty, by automatically stashing (including untracked files) and restoring local changes afterward.

Before this spec, `worktree.Merge` refused dirty roots:

> `colony root has uncommitted changes — commit or stash before merge`

That blocked an otherwise deliberate approve action for a common solo-developer state. `worktree.Merge` now autostashes by default and restores after a successful merge.

## Goals

- Make final merge on approve succeed when the colony root has uncommitted (including untracked) changes.
- Autostash **by default** — no opt-in flag for MVP.
- Auto-restore local changes after a successful merge (`stash pop`).
- Surface stash outcomes clearly in CLI / Console approve messages.
- Keep merge-target branch selection unchanged.

## Non-Goals (MVP)

- Opt-out / config flag to disable autostash.
- Changing how the merge target branch is chosen (`gitroot.DefaultBranch` / HEAD vs true `main`).
- Returning to a previous branch after merge (existing flow already stays on the pre-merge HEAD branch in the attached-HEAD case).
- Auto-resolving `stash pop` conflicts.
- Rolling back a successful merge because stash restore failed.
- Autostash for any path other than `worktree.Merge` (today the sole caller is `review.Approve` for isolated final gates).
- Root proposal approve (R1) — still no worktree merge.

## System Context

| Primitive | Location / behavior |
| --------- | ------------------- |
| Approve → merge | `internal/review/gate.go` → `worktree.Merge` when `ShouldMergeOnApprove` and worktree exists |
| Dirty root | `internal/worktree/merge.go` → `hasUncommittedChanges` via `git status --porcelain`; if dirty, `autostash` before checkout |
| Autostash | `git stash push --include-untracked -m "paseka: autostash before merge <traceId>"` |
| Restore | After successful merge + worktree removal → `stash pop`; outcome on `MergeResult.StashOutcome` |
| Merge steps | autostash (if needed) → `checkout` default branch → `merge --no-ff` → remove trace worktree → `stash pop` (if stashed) |
| “Default branch” | `gitroot.DefaultBranch` = current `HEAD` branch name; detached HEAD falls back to `main` |
| Stash outcomes | `StashOutcome` enum on `worktree.MergeResult`; threaded through `review.ApproveResult` |
| User messages | `internal/review/message.go` — `ApproveMessage` (Console) and `CLIApproveMessage` (`paseka proposal approve`) |
| Root approve (R1) | No merge (unchanged; out of scope) |
| Console approve API | `message` reflects stash outcome; no dedicated `stashOutcome` JSON field (MVP) |

## Decisions

### 1. Default-on autostash

If the colony root is dirty before merge, run autostash automatically. No CLI/API flag and no colony config toggle in MVP.

### 2. Include untracked files

Use `git stash push --include-untracked` (`-u`) so porcelain-dirty trees that are only untracked still clear, and incoming merge paths cannot collide with leftover untracked files.

Use a clear stash message, e.g. `paseka: autostash before merge <traceId>`, so a left-behind stash is findable.

### 3. Restore only after successful merge

| Phase | Behavior |
| ----- | -------- |
| Before merge | If dirty → `stash push -u`; record that a stash was created |
| Merge fails after stash | **Do not** pop. Return error mentioning the stash (message / tip to `git stash pop` or `git stash list`) |
| Merge succeeds | `stash pop` |
| `stash pop` conflicts | Merge remains success (commit SHA, worktree removed, `task.completed`). Warn that local restore conflicted and needs manual resolution |
| Clean tree | No stash; behavior unchanged |

### 4. Merge success vs stash restore

Approve/merge success is defined by the merge commit and worktree cleanup, not by a clean stash restore. Local WIP must not force a merge rollback.

### 5. Branch targeting unchanged

Do not change merge target selection in this work. Autostash wraps the existing checkout + merge sequence only.

### 6. Outcome reporting

`worktree.MergeResult` carries a `StashOutcome` enum:

| Outcome | Meaning |
| ------- | ------- |
| `none` | Root was clean; no stash |
| `restored` | Stashed, merge ok, pop ok |
| `left_on_failure` | Stashed, merge failed; stash still present |
| `restore_conflicted` | Merge ok; pop conflicted |

CLI / Console approve `message` reflects these cases:

- clean: “Task approved and worktree merged.” / “Approved and worktree merged.”
- restored: “… Local changes were restored.”
- restore conflicted: merge ok + warning to resolve stash/working-tree conflicts manually

Console approve API exposes the outcome only via `message` (no dedicated JSON field in MVP). `MergeResult.StashOutcome` is available internally so callers build messages without parsing git stderr.

## Implementation

Shipped in `internal/worktree/merge.go`:

1. If dirty → `stash push -u -m "paseka: autostash before merge <traceId>"`
2. Run existing checkout + merge + worktree removal
3. On merge failure after stash → return error including stash guidance; set outcome `left_on_failure`
4. On merge success after stash → `stash pop`; set `restored` or `restore_conflicted` (detected via `git diff --diff-filter=U`)

Threaded through:

- `internal/review/gate.go` — `ApproveResult.StashOutcome`
- `internal/review/message.go` — Console and CLI message composition
- `internal/console/review.go` — approve response `message`
- `cmd/paseka/nats.go` — `paseka proposal approve` output

Tests (`internal/worktree/merge_test.go`, `internal/review/message_test.go`):

- dirty tracked file → merge succeeds, changes restored
- dirty untracked file → same with `-u`
- merge conflict after stash → error, stash remains
- successful merge + pop conflict → success + `restore_conflicted`
- clean tree → unchanged path (`none`)

## Acceptance Criteria

- [x] Approving an isolated final merge gate with a dirty colony root (tracked and/or untracked) completes the merge without requiring a manual stash.
- [x] After a successful merge, local changes are restored when pop is clean; the user-facing message says so.
- [x] If merge fails after autostash, approve fails, stash remains, and the error tells the user how to recover.
- [x] If merge succeeds but pop conflicts, approve still reports merge success with a clear warning.
- [x] Clean-root approve behavior and root (R1) approve semantics are unchanged.
- [x] No new opt-in/opt-out flag ships in this MVP.

## Open Questions

None for MVP. Follow-ups (separate specs if needed):

- True default-branch resolution (`origin/HEAD` / `main`) vs merge-into-current-HEAD.
- Explicit `--no-autostash` / colony config if a real need appears.
- Dedicated `stashOutcome` field on Console approve API JSON if clients need structured access.
