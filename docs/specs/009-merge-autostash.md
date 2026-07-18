# Spec 009: Merge Autostash on Approve

## Status

**Agreed — not implemented.** Shared design from flight trail `trace-019f73945163fe35`.

## Purpose

When a human approves an isolated final merge gate, merge the trace worktree into the colony root even if the root working tree is dirty, by automatically stashing (including untracked files) and restoring local changes afterward.

Today `worktree.Merge` refuses dirty roots:

> `colony root has uncommitted changes — commit or stash before merge`

That blocks an otherwise deliberate approve action for a common solo-developer state.

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

## Current System Context

| Primitive | Location / behavior |
| --------- | ------------------- |
| Approve → merge | `internal/review/gate.go` → `worktree.Merge` when `ShouldMergeOnApprove` and worktree exists |
| Dirty check | `internal/worktree/merge.go` → `hasUncommittedChanges` via `git status --porcelain` |
| Merge steps | `checkout` “default branch” → `merge --no-ff` → remove trace worktree |
| “Default branch” | `gitroot.DefaultBranch` = current `HEAD` branch name; detached HEAD falls back to `main` |
| Root approve (R1) | No merge (unchanged; out of scope) |
| Console / CLI approve | `internal/console/review.go`, `paseka proposal approve` — human-readable success messages |

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

Extend internal `worktree.MergeResult` with a stash outcome enum (names illustrative):

| Outcome | Meaning |
| ------- | ------- |
| `none` | Root was clean; no stash |
| `restored` | Stashed, merge ok, pop ok |
| `left_on_failure` | Stashed, merge failed; stash still present |
| `restore_conflicted` | Merge ok; pop conflicted |

CLI / Console approve `message` must reflect these cases explicitly, for example:

- clean: existing “Task approved and worktree merged.”
- restored: mention that local changes were restored
- restore conflicted: merge ok + warning to resolve stash/working-tree conflicts manually

A dedicated public JSON field on the approve API is optional for MVP if the message is unambiguous; the structured field on `MergeResult` is required so callers can build that message without parsing git stderr.

## Implementation Outline

1. In `internal/worktree/merge.go`:
   - If dirty → `stash push -u -m "paseka: autostash before merge <traceId>"`
   - Run existing checkout + merge + worktree removal
   - On merge failure after stash → return error including stash guidance; set outcome `left_on_failure`
   - On merge success after stash → `stash pop`; set `restored` or `restore_conflicted`
2. Thread stash outcome from `MergeResult` into `review.Approve` / Console / CLI message composition.
3. Tests:
   - dirty tracked file → merge succeeds, changes restored
   - dirty untracked file → same with `-u`
   - merge conflict after stash → error, stash remains
   - successful merge + pop conflict → success + `restore_conflicted`
   - clean tree → unchanged path (`none`)

## Acceptance Criteria

1. Approving an isolated final merge gate with a dirty colony root (tracked and/or untracked) completes the merge without requiring a manual stash.
2. After a successful merge, local changes are restored when pop is clean; the user-facing message says so.
3. If merge fails after autostash, approve fails, stash remains, and the error tells the user how to recover.
4. If merge succeeds but pop conflicts, approve still reports merge success with a clear warning.
5. Clean-root approve behavior and root (R1) approve semantics are unchanged.
6. No new opt-in/opt-out flag ships in this MVP.

## Open Questions

None for MVP. Follow-ups (separate specs if needed):

- True default-branch resolution (`origin/HEAD` / `main`) vs merge-into-current-HEAD.
- Explicit `--no-autostash` / colony config if a real need appears.
