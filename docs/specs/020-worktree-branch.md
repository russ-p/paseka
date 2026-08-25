# Spec 020: Worktree branch name (`worktree.branch`)

## Status

**Implemented**

`INSIGHT/worktree.branch` names and renames the isolated trace worktree git ref; default remains `paseka/<traceId>`.

## Problem Statement

Isolated code work lives in a colony-managed git worktree. The directory is `.paseka/worktrees/<traceId>/` and the branch is always `paseka/<traceId>`. That id is unique and merge/diff can find it, but it is useless as a git name: a Scout Bee that already classified the work as a feature or hotfix cannot leave `feature/live-bees-header` or `hotfix/windows-path` for the Builder Bee, merge preview, or later human git history.

Beekeepers scanning local branches or merge-diff see opaque `paseka/trace-…` refs. There is no contract to set or correct the name after the worktree already exists.

## Solution

Introduce `INSIGHT/worktree.branch` — an operational, last-write-wins git branch name for the trace worktree.

Scout (and other planner bees) emit a conventional name when classifying or planning (`feature/<slug>`, `hotfix/<slug>`, or another valid ref). Hive Runtime applies it when ensuring the worktree (create with `-b`) or, if the worktree already exists for that trace, by renaming the current branch. The worktree **path** stays `.paseka/worktrees/<traceId>/`. Merge, merge-diff, and the machine-local worktree registry use the resolved branch.

Default when no valid insight exists remains `paseka/<traceId>`.

Bee-language prompts say **worktree branch**; the wire kind stays **`worktree.branch`**.

## User Stories

1. As a beekeeper, I want isolated work on a readable git branch, so that `git branch` and merge preview match how I think about the change.
2. As a beekeeper, I want the default `paseka/<traceId>` when nobody named the branch, so that trails without a planner emit still isolate and merge as today.
3. As a Scout Bee on intake, I want to publish a worktree branch when I classify `plan` or `triage`, so that the Builder Bee’s worktree is born with `feature/…` or `hotfix/…`.
4. As a Scout Bee with `decision=plan`, I want prompt guidance to use a `feature/<slug>` name derived from the entry or trail title, so that feature work is named consistently.
5. As a Scout Bee with `decision=triage`, I want prompt guidance to use a `hotfix/<slug>` (or `fix/<slug>`) name, so that bug work is named consistently.
6. As a Scout Bee on `grill` / `clarify` / `reject`, I want branch emit to be optional, so that I do not name a worktree that may never be created.
7. As a Drone Bee publishing a breakdown, I want to set or refine the worktree branch for the whole trail, so that a later isolated builder still gets a good name.
8. As a Builder Bee with `worktree: true`, I want `Ensure` to create the branch from the latest insight when the worktree does not exist yet, so that I never start on a throwaway `paseka/<traceId>` if Scout already named it.
9. As a Guard Bee reviewing an isolated proposal, I want to reuse the same worktree and branch, so that review stays on the named ref.
10. As a beekeeper in interactive `bee chat` with `worktree: true`, I want the same ensure/rename rules as AFK, so that HITL isolated sessions are not a second naming scheme.
11. As a beekeeper, I want the worktree directory to stay `.paseka/worktrees/<traceId>/`, so that adapters, runs audit, and gitignore paths do not churn when the git branch is renamed.
12. As a bee, I want a later `worktree.branch` emit to rename an existing trace worktree branch, so that a sharper name can replace `paseka/<traceId>` or a weak first slug (last-write-wins).
13. As a beekeeper, I want rename to happen as soon as the insight is applied if the worktree already exists, so that I do not wait for the next bee dispatch.
14. As a bee, I want empty or whitespace branch emits rejected, so that the name cannot be cleared by accident in v1.
15. As a bee, I want invalid git refs rejected (`check-ref-format`, spaces, `..`, leading `-`, etc.), so that `worktree add` / `branch -m` cannot be fed garbage.
16. As a bee, I want names that collide with `HEAD`, the colony default branch (`main`/`master`/resolved default), or `detached` sentinels rejected, so that isolated work cannot target the beekeeper’s primary checkout.
17. As a bee, I want overly long names rejected, so that refs stay usable in UI and git.
18. As a beekeeper, I want create/rename to **fail closed** when the desired branch already exists in the repo and is not this trace’s current worktree branch, so that we never silently check out someone else’s `feature/foo` or fall back to detached HEAD.
19. As a beekeeper, I do **not** want today’s “branch already exists → add worktree without `-b`” fallback when a named insight is in play, so that detached HEAD is not an implicit success.
20. As a beekeeper, I want collision and rename failures visible on the Flight Path (validation error or a failed ensure/side-effect), so that I can pick another name instead of discovering a detached worktree at merge.
21. As a downstream bee, I do not want `worktree.branch` injected into `{{.Insights}}`, so that operational git metadata does not crowd prompt memory.
22. As a colony author, I want no bee YAML `dispatch: direct` subscribers on this kind, so that naming does not route work (not a SIGNAL trigger).
23. As a colony author, I want no completion-contract `required` and no runtime auto-synthesis of `worktree.branch` in v1, so that missing names stay on the default `paseka/<traceId>`.
24. As a beekeeper approving an isolated final gate, I want merge and merge-diff to use the registry/current worktree branch, so that `feature/…` merges instead of looking for a stale `paseka/<traceId>`.
25. As a beekeeper, I want Queen Console merge-diff to show the actual branch name, so that the preview matches git.
26. As a beekeeper on the trace detail (or worktree registry surfaces), I want to see the current branch when a worktree exists, so that Console and `state.json` do not disagree.
27. As a beekeeper, I want the machine-local worktree registry `Branch` field updated on create and rename, so that merge after a restart still finds the ref.
28. As a Medic Bee / `paseka doctor` path, I want cleanup to still key worktrees by `traceId` and path, so that a renamed branch does not orphan the registry entry.
29. As a beekeeper, I want same-name last-write-wins emits to be no-ops, so that duplicate Scout emits do not fail.
30. As a beekeeper, I want rename to work while the worktree is checked out (including an in-flight builder cwd), so that we do not require tearing down the worktree to retitle the branch.
31. As a hivewright, I want Scout `publishes` to declare `INSIGHT/worktree.branch`, so that doctor/topology show the contract.
32. As a beekeeper using Forage Cues `feature` / `hotfix`, I want Scout to still choose the prefix from classification (not from cue id alone), so that a mis-fired cue can be corrected in `feature.classified`.
33. As an implementer, I want one resolve helper (latest insight, else default `paseka/<traceId>`), so that Ensure, rename side-effect, merge, and merge-diff cannot diverge.
34. As an implementer, I want ledger apply to ignore this kind (no task-queue effect), so that the task ledger stays about nectar tasks.
35. As a beekeeper, I want root proposals (`worktree: false`) to ignore this kind, so that hivewright edits on colony checkout never create or rename a trace worktree just because a branch was named.
36. As a bee, I want slugs to be git-safe (lowercase kebab after sanitizing the title), so that `trace.title` prose is not copied verbatim into a ref.
37. As a beekeeper, I want a second trace that requests the same `feature/foo` to fail rather than steal the first trace’s branch, so that traces stay isolated.
38. As a beekeeper, I want `paseka/<traceId>` still reserved as the anonymous default, so that unnamed trails do not collide with each other.

## Implementation Decisions

### Kind and payload

- New operational `INSIGHT` kind: `worktree.branch`.
- Payload shape: `{ "kind": "worktree.branch", "branch": "<git-ref>" }` only — no `taskId`, no `prefix` field in v1 (prefix lives inside `branch`).
- Envelope authorship (`agentId`, `createdAt`, `seq`) is enough for audit.
- Last-write-wins per trace: latest event by `createdAt`, then `seq`.
- Not projected into `{{.Insights}}` (same class as `trace.title` / `trace.summary` / `task.plan`).
- Does **not** drive bee routing or completion contracts.
- Runtime does **not** auto-emit `worktree.branch` in v1.
- Task ledger reducer ignores the kind (no `changed` from this event alone).

### Validation (emit)

- `branch` required after trim; empty/whitespace rejected (no clear/sentinel in v1).
- Maximum length: **120** characters after trim (aligned with `trace.title`).
- Must pass `git check-ref-format --allow-onelevel` semantics for a branch name (no `..`, control chars, leading/trailing `.` or `/`, `@ {`, trailing `.lock`, etc.). Prefer validating with git rather than a homemade regex.
- Reject case-insensitive match against: `HEAD`, `main`, `master`, and the resolved colony **default branch**.
- Do not require a `feature/` / `hotfix/` prefix at the protocol layer; prompts guide conventional names. Any other valid ref (`docs/…`, `paseka/…`) is allowed.
- Do **not** accept names that would be interpreted as a remote-tracking or reflog path; keep to a single branch ref.

### Resolve order

1. Latest valid `INSIGHT/worktree.branch` for the trace (already persisted; invalid emits never land).
2. Else default `paseka/<safeTraceId>` where `safeTraceId` replaces `/` and spaces with `-` (today’s `branchName`).

Resolve is used at Ensure, rename side-effect, merge, and merge-diff. Do not re-sanitize a successfully validated insight (keep the bee’s spelling, including slashes).

### Apply: create

When a bee or isolated-proposal path needs a worktree and the path is not yet a git worktree:

- Create with `git worktree add -b <resolved> <path> HEAD` as today, but `<resolved>` from the helper above.
- On “branch already exists”: **do not** retry without `-b`. Return an error (fail closed).
- Register machine-local worktree state with `TraceID`, `Path`, `BaseSHA`, `Branch` = resolved name.

### Apply: reuse and rename

When the path is already this trace’s worktree:

- Read the current branch (`rev-parse --abbrev-ref HEAD` in the worktree).
- If current equals resolved (including default), reuse as today.
- If current differs: `git branch -m <resolved>` from the worktree (rename the checked-out branch). Then update the registry `Branch` field **in place** (today’s register-if-new is not enough).
- If `-m` fails because `<resolved>` exists: fail closed; leave the old branch and registry unchanged.
- Detached HEAD with an empty registry branch: treat as “unnamed”; attempt to checkout/create the resolved branch only if it does not exist; if it exists and is not already this worktree, fail closed. (Do not revive the silent detached fallback for named insights.)

### Apply: insight after worktree exists

- On successful persist of `INSIGHT/worktree.branch`, Hive Runtime runs a post-apply side effect (same family as other reactor side effects, not a bee dispatch): if a worktree is registered/present for that `traceId`, run the rename path immediately.
- If no worktree yet, the side effect is a no-op; the next Ensure uses the insight.
- Root-only bees and `code.proposal.root` paths never Ensure or rename.

### Registry and merge

- Merge and merge-diff already prefer registry `Branch` over recomputed `paseka/<traceId>`. After this spec they must prefer: live worktree HEAD branch if present, else registry `Branch`, else resolve helper. Live HEAD wins if registry is stale.
- Purge/unregister still keys by `traceId` + path.

### Prompts and bee config

- Scout intake (and Drone breakdown) gain emit guidance next to `trace.title`: when planning isolated work, emit one `worktree.branch`.
- Naming guidance (prompt, not protocol): slug from title/body, kebab-case, `feature/` for `decision=plan`, `hotfix/` for `decision=triage`.
- Scout `publishes` adds `INSIGHT` / `worktree.branch`. Not required on completion contracts.
- Optional `{{.WorktreeBranch}}` on prompt context (resolved name, default if unset) so planners see the current ref; not required for v1 if emit partials are enough.

### Console / CLI

- Merge-diff payload already has `branch`; it must show the resolved/live name.
- Trace detail should expose the worktree branch when a worktree exists (no new INSIGHT timeline highlight — operational, like title/summary).
- No new Queen Shell command in v1 (`paseka worktree` list/clean remains later architecture).

### Collision and concurrency

- Branch names are **repo-global**, not namespaced by trace. Two traces cannot share a branch.
- In-flight adapter cwd in the worktree: rename of the checked-out branch is allowed; do not require killing the bee first.
- Do not rename the worktree directory.

## Testing Decisions

Good tests assert git and registry **behavior** visible outside the helper: created ref name, reuse without extra worktrees, rename updates HEAD and `state.json`, collisions error, invalid payloads rejected at validate, ledger unchanged, merge-diff reports the new name. Do not assert internal function names.

Cover at least:

- Worktree ensure: default `paseka/<traceId>` with no insight; create `-b` from latest insight; reuse same path; collision fails (no detached HEAD).
- Rename: existing default branch → insight rename; duplicate same-name emit is no-op; colliding target fails and leaves old name.
- Protocol validate: empty, too long, `main`/`HEAD`, invalid ref characters.
- Insight projection: kind excluded from `{{.Insights}}`.
- Ledger: applying `worktree.branch` does not add tasks or ready-queue.
- Reactor/runtime: insight before first Ensure; insight after Ensure triggers rename without a builder dispatch; `worktree: false` / root proposal does not Ensure.
- Merge / merge-diff: uses renamed branch (prior art: worktree merge and merge-diff tests).

Prior art: worktree ensure/merge/diff tests; `trace.title` validate + resolve tests; reactor post-apply side effects (review/invite); protocol insight-kind validation.

## Out of Scope

- Renaming or relocating `.paseka/worktrees/<traceId>/` (path stays trace-scoped).
- Pushing the branch to `origin` or opening a GitHub PR automatically.
- Per-task branches (one branch per `taskId`); v1 is one branch per Flight Trail worktree.
- Clearing a custom name back to `paseka/<traceId>` via empty payload (no sentinel in v1).
- Required completion contract or runtime auto-synthesis from `trace.title` / cue id.
- `SIGNAL` kind or bee subscribers that dispatch on branch naming.
- Bee-language wire kinds (`trail.branch`).
- Queen Shell `paseka worktree` list/clean (still later).
- Reusing another trace’s worktree/branch after kill (see backlog: graft worktree to a new `traceId`).
- Namespacing all custom names under `paseka/` (allowed but not required).
- Storing the desired branch on `task.plan` or `feature.classified` instead of a dedicated insight.

## Further Notes

- **Why INSIGHT, not SIGNAL:** naming is trail metadata with a git side effect, analogous to `trace.title`, not a trigger that should have `dispatch: direct` bees. Runtime applies it; bees do not subscribe.
- **Why not a field on `task.plan`:** plans describe nectar tasks; several tasks share one isolated worktree. Branch is trace-scoped.
- **Default vs conventional:** keeping `paseka/<traceId>` as anonymous default avoids collisions between unnamed trails. Scout should emit conventional names when it intends isolated implementation.
- **Fail closed vs detached HEAD:** today’s ensure fallback exists for an already-created `paseka/<traceId>` on retry. Named insights must not inherit that fallback; retry of the **same** default name when the worktree is missing but the branch exists is still an error in v1 (beekeeper/doctor cleanup), not silent detach.
- Related: [insight kinds](../reference/insight-kinds.md), [architecture overview](../architecture/overview.md) (worktree path + branch), [008](008-code-proposal-workspaces.md), [011](011-trace-title.md), [012](012-trace-summary.md), [017](017-console-diff-review.md) (merge-diff `branch` field).
