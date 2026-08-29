# Spec 023: Queen Console Git

## Status

**(Approved)**
Queen Console Git tab for the colony clone: status vs `origin`, explicit fetch/push/ff-only pull, worktrees, leftover local branches. Designed for homelab apiary (Gitea + inbound webhook pull). Operator push is not folded into review approve.

## Problem Statement

On a laptop apiary, after a Beekeeper approves an isolated final merge gate, the merge commit lives in the same clone they already `git push` from. On a homelab apiary the clone is on the server: Queen Console merges into that working copy, a sidecar may `git pull` when Gitea receives a push, and the laptop never sees the new default-branch commits until someone publishes them.

Today there is no Console surface for that clone. Beekeepers SSH or `docker exec` to see whether local `main` is ahead of `origin`, to push, to fetch when the webhook lagged, or to delete `feature/…` / `paseka/<traceId>` refs that `worktree.Merge` leaves behind after removing the worktree. Trace detail shows one live worktree; there is no colony-wide worktree list (deferred in [002](./002-queen-console-mvp.md)).

They do not want auto-push on approve, a Gitea/GitHub clone, or a desktop git IDE. They want a trusted, single-machine Git tab that treats Paseka as the local merger and `origin` as the replica to publish to — using the same system `git` (HTTPS credential helper, e.g. Gitea `tea`) that already works from a non-interactive `docker exec`.

## Solution

Add a Queen Console **Git** tab and a compact header plaque. The tab has three blocks:

1. **Colony root vs origin** — current branch, short SHA, dirty flag, ahead/behind `origin/<default>`, last fetch age when known, unpublished commits; **Fetch** (remote-tracking only); **Push** default branch (explicit, never `--force`); **Pull** fast-forward only as a backup when an inbound webhook did not update the clone.
2. **Worktrees** — colony-managed isolated checkouts (path, branch, `traceId`, dirty), with prune of orphans. This is the dedicated worktrees surface deferred from [002](./002-queen-console-mvp.md).
3. **Branches** — local refs with HEAD / worktree / merged flags; delete one merged unused branch; batch-delete leftover merged `paseka/*` and conventional feature/hotfix names.

Reviews warn before Approve when `origin` is ahead of the merge target so a sidecar pull cannot collide with a merge based on a stale `HEAD`. Push (and Console merge commits) default to skipping git hooks (`--no-verify`); a control can re-enable hooks. Mutations are separate HTTP POSTs from approve; a failed push does not roll back `task.completed`.

## User Stories

1. As a Beekeeper on a homelab apiary, I want a **Git** tab in Queen Console, so that I can operate the colony clone without SSH.
2. As a Beekeeper, I want a compact header plaque for git sync (ahead/behind/dirty/synced), so that unpublished merges are visible from any tab.
3. As a Beekeeper, I want to click the git plaque (and use Enter/Space) to open the Git tab, so that navigation matches Host → System and Live bees → Runs/Sessions.
4. As a Beekeeper, I want the Git tab reachable from the tab list, so that I do not have to use the plaque.
5. As a Beekeeper, I want current branch and short HEAD SHA, so that I can confirm Console is looking at this clone.
6. As a Beekeeper, I want a dirty flag when the colony root has uncommitted or untracked changes, so that I know Pull and merge autostash may be involved ([009](./009-merge-autostash.md)).
7. As a Beekeeper, I want ahead and behind counts versus `origin` on the default branch, so that I can tell “need Push” from “sidecar/webhook lag” from “in sync”.
8. As a Beekeeper, I want last-fetch age when git can infer it, so that stale remote-tracking refs are obvious; omit the field when unknown rather than inventing a time.
9. As a Beekeeper, I want Fetch to update remote-tracking refs only, so that status can refresh without competing with a webhook `git pull`.
10. As a Beekeeper, I want status GET **not** to fetch on every poll, so that Console does not hammer Gitea.
11. As a Beekeeper, I want Fetch available when Hive runtime or NATS is down, so that clone ops do not depend on the bus.
12. As a Beekeeper with no `origin` remote, I want Push/Pull/Fetch disabled with a clear reason, so that a laptop-only repo is not an error.
13. As a Beekeeper after an isolated final approve, I want an explicit **Push** of the default branch, so that Gitea and the laptop receive the merge commit.
14. As a Beekeeper, I do **not** want approve to push, so that merge success and remote publish stay different failure domains ([review.Approve](./009-merge-autostash.md) already completes the trail before any network).
15. As a Beekeeper, I want Push to send only the default branch (what merge just updated), so that Paseka stays the merger and we do not publish leftover worktree branches instead of `main`.
16. As a Beekeeper, I want Push refused when the branch is behind `origin` (non-fast-forward), so that Console never `--force` and never silently merge/rebase with origin.
17. As a Beekeeper, I want Push refused when there is nothing to push, so that the button is not a no-op surprise (or it reports up-to-date without failing the trail).
18. As a Beekeeper, I want a short list of unpublished commits (`origin/<default>..HEAD`) before Push, so that I see what will land on Gitea.
19. As a Beekeeper, I want Push never to use `--force` or `--force-with-lease`, so that a protected or shared default branch cannot be rewritten from Console.
20. As a Beekeeper, I want Push to use system `git` with the process environment of `paseka console`, so that HTTPS credential helpers (e.g. Gitea `tea`) work the same as `docker exec` / a host shell.
21. As a Beekeeper, I want Paseka **not** to implement Gitea/GitHub OAuth or store git tokens, so that apiary secrets stay in git’s helper and tea config.
22. As a Beekeeper, I want Push to default to `--no-verify`, so that husky/`pre-push` that need `npm` on an interactive PATH cannot fail a homelab publish.
23. As a Beekeeper, I want an explicit control to run git hooks on Push (unset `--no-verify`), so that a colony that relies on `pre-push` can opt in.
24. As a Beekeeper, I want hook skip to set `HUSKY=0` on the git child as well when skipping, so that husky does not still run if a hook ignores `--no-verify`.
25. As a Beekeeper, I want git stderr (including husky skip/fail lines) in the Console error or result message, so that a hook problem is not mistaken for a tea/auth failure.
26. As a Beekeeper, I want isolated final-gate merge commits from Console/CLI approve to default to `--no-verify` as well, so that `commit-msg` / husky on merge cannot block HITL merge in the same container.
27. As a Beekeeper, I want Pull to be **fast-forward only** of the default branch, so that Console is not a second merger beside Paseka approve and the webhook sidecar.
28. As a Beekeeper, I want Pull labeled as a backup when inbound sync (webhook sidecar) did not run, so that I do not treat Console Pull as the primary Gitea → clone path.
29. As a Beekeeper, I want Pull refused when the update would not be a fast-forward, so that diverged history is fixed over SSH, not from the browser.
30. As a Beekeeper, I want Pull refused when the colony root is dirty, so that webhook-style ff cannot mix with hivewright WIP.
31. As a Beekeeper, I want Pull refused while a merge/rebase/cherry-pick is in progress on the root, so that we do not nest git states.
32. As a Beekeeper, I want Pull refused when Live bees are using the colony root as cwd (root / hivewright path), so that a ff does not rewrite files under an in-flight adapter.
33. As a Beekeeper, I want Pull **not** blocked solely because isolated worktrees exist, so that updating `main` does not require draining every Flight Trail.
34. As a Beekeeper on Reviews for an isolated final gate, I want a warning when `origin` is ahead of the local default branch, so that I fetch/pull before Approve and avoid merging onto a stale `HEAD` that the sidecar will then fast-forward into.
35. As a Beekeeper, I want that warning to use last known remote-tracking state (and a Fetch control or copy), so that Reviews does not have to auto-fetch.
36. As a Beekeeper, I want local branches listed with name, whether it is HEAD, linked worktree/`traceId` when any, whether it is merged into the default branch, and last commit summary, so that leftover refs after merge are visible.
37. As a Beekeeper, I want to delete one local branch that is merged and not checked out in a live worktree and is not the default branch, so that I can clean a single leftover `feature/…` without SSH.
38. As a Beekeeper, I want delete refused for the default branch, HEAD, unmerged branches, and branches with a live worktree, so that Console cannot drop in-flight isolated work.
39. As a Beekeeper, I want leftover merged refs called out (`paseka/*` and merged conventional `feature/` / `hotfix/` / `fix/` names without a live worktree), so that a week of trails does not require hunting the full branch list.
40. As a Beekeeper, I want a batch “delete N merged leftovers” with a confirm listing the names, so that cleanup is one action.
41. As a Beekeeper, I want batch delete to skip and report any name that fails the same guards as single delete, so that a mixed list cannot remove a live worktree branch.
42. As a Beekeeper, I want all active colony-managed worktrees on the Git tab (path, branch, `traceId`, base SHA, dirty), so that I do not rely only on Dashboard count or one trace’s detail.
43. As a Beekeeper, I want a worktree row to link to the Flight Trail when `traceId` is set, so that I can jump to Traces without copying ids.
44. As a Beekeeper, I want prune of orphan worktrees (git worktree gone or path missing, registry stale, or git worktree list entries under `.paseka/worktrees/` with no registry/`traceId`), so that doctor-like cleanup is possible from Console.
45. As a Beekeeper, I want prune **not** to `worktree remove --force` a registered live worktree that still has a checkout, so that an in-flight builder is not deleted from this button (use reject/merge/kill paths).
46. As a Beekeeper, I want Git status and mutations to work without NATS, so that I can publish a merge after the bus is down.
47. As a Beekeeper, I want auth/credential failures returned as git’s message, so that a broken `tea` helper is diagnosable from Console.
48. As a Beekeeper, I want no interactive username prompt: git child stdin is not a TTY, so that a missing helper fails closed instead of hanging the HTTP request.
49. As a Beekeeper on a laptop apiary, I want the same Git tab, so that the feature is not homelab-only even if Push is used less often.
50. As a Beekeeper, I want Telegram and Queen Shell unchanged in this spec, so that git ops stay Console-first.
51. As a Beekeeper, I want no new domain bus events for push/fetch/pull in v1, so that the task ledger does not record git replica sync.
52. As a platform contributor, I want SPA tests to assert the Git tab, plaque, and API paths exist, so that the surface cannot silently disappear.
53. As a Beekeeper, I want documentation after ship (Console / homelab) to say inbound ff is typically a webhook sidecar and Console Push is outbound, so that operators do not build a second pull daemon in Paseka.

## Implementation Decisions

### 1. Product scope

- Queen Console only for this spec (HTTP + SPA). No `paseka git` CLI, no Telegram commands, no auto-push after `review.Approve`.
- Operate the **colony root git repo** of the Console process (same root as merge). Not a multi-remote control plane; default remote name is `origin` when present.
- Paseka remains the local merger ([008](./008-code-proposal-workspaces.md), [009](./009-merge-autostash.md)). Console Git publishes or fast-forwards that clone; it does not open PRs or push worktree branches as a substitute for merge.
- Inbound sync from Gitea/GitHub stays an operator sidecar (webhook `git pull`). Console **Fetch** refreshes tracking refs; Console **Pull** is ff-only backup.

### 2. Header plaque and Git tab

- Fourth header plaque **Git** (alongside Hive runtime, Live bees, Host): compact sync summary (e.g. default-branch name plus ahead `↑N`, behind `↓N`, `dirty`, or `synced`; `no origin` when unset). Idle/error styling when status GET fails.
- Click / Enter / Space opens the **Git** tab. Tab is also in the tab list.
- Git tab layout, top to bottom: **Colony root vs origin**, **Worktrees**, **Branches** (leftovers highlighted + batch delete).
- Refresh control on the tab; plaque may poll on the existing header timer using **GET status only** (no implicit fetch).

### 3. Status snapshot (GET)

- One read-only JSON snapshot for plaque + tab: current branch, HEAD short/full SHA, dirty (porcelain including untracked), default branch name, origin URL if any, ahead/behind vs `origin/<default>` when remote-tracking ref exists, last fetch age best-effort, capped unpublished commit list (`origin/<default>..HEAD`: sha, subject), worktree rows, local branch rows with leftover flags.
- Ahead/behind use already-fetched remote-tracking refs. Missing `origin/<default>` → behind/ahead omitted or zero with a note to Fetch.
- Do not run `git fetch` inside GET.
- Independent of NATS and Hive runtime (same idea as [022](./022-console-system-info.md)).
- Truncate commit lists (about 20). Do not return full diffs.

### 4. Fetch / Push / Pull (POST)

- Separate endpoints from review approve. Success/failure of Push does not write `task.completed` or roll back a merge.
- **Fetch:** `git fetch origin` (or configured origin). No merge, no checkout.
- **Push:** `git push origin <defaultBranch>` (the resolved default branch, typically current `main` after merge). No `--force`. Refuse if behind or if HEAD is not that branch (stay on default branch for v1). Optional body flag `runHooks` default **false** → `--no-verify` plus `HUSKY=0` on the child env when skipping.
- **Pull:** `git pull --ff-only origin <defaultBranch>` (or fetch + ff-only merge). Refuse dirty root, in-progress merge/rebase, non-ff, and live bees whose workspace is colony root (reuse Live bees / agent list + bee `worktree: false` / root cwd — fail closed if cwd cannot be classified and a live bee exists). Isolated worktrees do not block Pull.
- Time-bound git children; return combined stderr/stdout on failure (cap size).
- Inherit `os.Environ()`; do not set `GIT_ASKPASS` or `GIT_TERMINAL_PROMPT=1`. Empty stdin.

### 5. Reviews: origin ahead

- Isolated final merge-diff / Reviews detail includes a sync hint: local default vs `origin/<default>` behind count when known.
- Warn copy: fetch or ff-only pull before approve if behind. Does not block Approve in v1 (operator may proceed); blocking approve is out of scope.
- Do not auto-fetch from the Reviews poll loop.

### 6. Hooks on merge approve

- `worktree.Merge` / approve merge commit path defaults to `--no-verify` (and `HUSKY=0` when skipping) so homelab containers without interactive Node PATH can merge. Same `runHooks` semantics as Push if a later API flag is added; v1 may apply skip-hooks unconditionally for merge to match Console Push default, or thread one Console/home setting — implementers pick one mechanism, not a second hidden default.
- Do not change autostash behavior ([009](./009-merge-autostash.md)).

### 7. Branches and leftovers

- List local branches (`refs/heads/*`). Mark: current, default, merged into default (`merge-base --is-ancestor`), worktree/`traceId` from registry + `git worktree list`.
- **Leftover:** merged, not default, no live worktree, and name is `paseka/*` **or** conventional `feature/` / `hotfix/` / `fix/` prefix.
- Delete: `git branch -d` (safe delete, merged only). Never `-D` in v1. Never delete default or a branch with a worktree. Batch leftover delete is repeated safe deletes + per-name results.

### 8. Worktrees

- Combine machine-local worktree registry with `git worktree list` and disk under `.paseka/worktrees/`. Show path, branch, `traceId`, base SHA, dirty in that checkout.
- **Prune orphans:** `git worktree prune`, unregister registry rows whose path is gone, remove leftover empty dirs when safe. Do not force-remove a worktree that is still a valid checkout registered to a trace (in-flight).
- Fulfills the dedicated worktrees page / list API deferred in [002](./002-queen-console-mvp.md). Trace detail worktree block remains.

### 9. Credentials and remotes

- HTTPS + git credential helper is the supported homelab path (documented: same user/`HOME`/`PATH` as a working non-interactive `git push`). SSH works if the Console process already has it; not a special case.
- Do not parse or mount `tea` config in Paseka.
- Single remote `origin`. No remote add/edit UI.

### 10. Modules

- Console HTTP handlers + SPA tab/plaque.
- Git helpers beside existing worktree/gitroot usage (status, fetch, push, ff-only pull, branch list/delete). Reuse worktree registry and merge-diff default-branch resolution.
- Review/merge-diff payload gains optional origin-ahead fields.
- No new NATS kinds, no colony.yaml git section. Optional later home `config.yaml` for skip-hooks is not required if API defaults suffice.

## Testing Decisions

Good tests assert **observable git and HTTP behavior**: status JSON matches a fixture repo (ahead/behind/dirty/no origin), fetch updates remote-tracking without moving HEAD, push dry-run or fake remote receives commits, push refuses behind without rewriting, pull `--ff-only` refuses dirty/diverged, branch `-d` refuses unmerged and worktree-backed names, leftover classification, prune removes stale registry rows only, approve/merge still succeeds with `--no-verify` default, SPA contains Git tab/plaque and API paths.

Do not assert tea, husky, or Gitea. Do not require network in unit tests (local `git init --bare` remotes).

Cover at least:

- Status: clean synced; ahead N; behind N; dirty; missing origin; missing `origin/<default>` after no fetch.
- Fetch updates `origin/<default>` SHA; GET does not fetch.
- Push: success on ahead; refuse behind; `--no-verify` on the invoked git args by default; `runHooks` true omits `--no-verify`.
- Pull: ff success; refuse dirty; refuse non-ff.
- Branches: leftover flags; delete merged ok; delete unmerged/default/worktree-linked refused; batch partial failure.
- Worktrees: list registry+git; prune orphan; do not remove live registered worktree.
- Reviews payload includes behind count when remote-tracking exists.
- Merge approve still creates merge commit when hooks would fail if not skipped (optional: repo with failing `commit-msg` hook).
- Static/SPA: Git tab, plaque, fetch/push/pull/delete/prune controls exist.

Prior art: `internal/worktree` merge/diff tests (temp repos), `internal/console` handler tests with git fixtures, `internal/console/static_test.go` for tab/API strings, [022](./022-console-system-info.md) plaque+tab pattern.

## Out of Scope

- Auto-push (or auto-fetch) on review approve; `git.pushed` / `git.push.failed` bus events.
- Pushing the trace worktree branch; opening Gitea/GitHub PRs ([020](./020-worktree-branch.md) remains out of scope for auto-PR).
- `--force` / rebase / merge commit with origin / `reset --hard` to origin.
- Checkout of a non-default branch on colony root; commit/amend/stage from Console (root R1 stays manual, [008](./008-code-proposal-workspaces.md)).
- Git credential UI, token env (`GITEA_TOKEN`) as a Paseka git auth path, embedding `tea`.
- Queen Shell `paseka git` / `paseka worktree list|clean` (architecture “later” CLI still later).
- Telegram git actions.
- Hard reset to match origin; stash pop/drop UI; full porcelain file list (see backlog).
- Disabling Push/branch-delete while Live bees run (Pull already guards root bees); broader mutation lock — backlog.
- Remote reachability probe beyond git command errors; commit hyperlinks into Gitea — backlog.
- Incoming `HEAD..origin/<default>` log beyond behind count (unpublished outbound list is in scope).
- Multi-host / other clones; Docker git; changing webhook sidecars.
- Blocking Approve when origin is ahead (warn only).

## Further Notes

- **Why not push inside Approve:** merge deletes the worktree and then publishes `task.completed`. Push is network + remote policy (protected branches, helper, non-ff). Same split as stash restore in [009](./009-merge-autostash.md): merge success is not defined by replica publish.
- **Why skip hooks by default:** homelab Console git has no login shell; nvm/sdkman are often only in `.bashrc`. Husky `pre-push` then skips or fails. Operator Push/merge is “publish reviewed work”; CI stays on Gitea. Opt-in `runHooks` is the escape hatch.
- **Webhook coexistence:** after Console Push, a sidecar ff-pull is typically a no-op. Console Pull is for missed hooks, not a second daemon.
- **Leftover branches:** `git worktree remove` does not delete the branch; merged `paseka/<traceId>` and named `feature/` refs accumulate. Safe `-d` plus leftover filter is the cleanup, not `branch -D`.
- Related: [002](./002-queen-console-mvp.md) (Console; worktrees page), [008](./008-code-proposal-workspaces.md), [009](./009-merge-autostash.md), [017](./017-console-diff-review.md), [020](./020-worktree-branch.md), [022](./022-console-system-info.md) (plaque/tab), [homelab deployment](../guide/homelab-deployment.md), [Backlog — Console Git follow-ups](../plans/backlog.md#console-git).
