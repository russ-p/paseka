# Spec 024: Pull-request delivery for isolated trails

## Status

**(Draft)**
Captures the design conversation: isolated Flight Trails can publish a pull request instead of merging into the colony clone’s default branch. Forge hosting is an external CLI script, not a bee adapter and not an in-process SDK. Gaps that were not locked are marked in Implementation Decisions.

## Problem Statement

Today an isolated trail ends with one HITL action: approve merges the trace worktree into the **local** default branch (`worktree.Merge`), removes the worktree, and completes the final gate. Origin is not involved. On a laptop that is often enough; the Beekeeper pushes `main` themselves. On a homelab apiary the clone sits on the server, `main` may be protected, CI runs on Gitea/GitHub, and other people (or the Beekeeper’s laptop) expect a **pull request**, not a merge commit that only exists until someone remembers to push default.

Queen Console Git ([023](./023-console-git.md)) still treats Paseka as the local merger and origin as a replica of `main`. It explicitly does not push worktree branches or open PRs. Bees already name the branch (`worktree.branch`) and describe the trail (`trace.title`, `trace.summary`), but there is no place for a real PR description, no publish of the isolated head, and no hosting client. Approving still means “merge into this clone,” which fights protected default branches and double-writes history if someone also opens a PR by hand.

The Beekeeper does not want Paseka to embed Gitea/GitHub OAuth or a vendor SDK. They already run `tea` / `gh` (or can write a bash wrapper). They want any other host to be a script that obeys a small contract.

## Solution

Add an opt-in **delivery policy** for isolated trails:

- **`local_merge` (default)** — today’s path: final-gate approve merges into the clone default branch ([008](./008-code-proposal-workspaces.md), [009](./009-merge-autostash.md)). Queen Console Git still publishes that default branch ([023](./023-console-git.md)).
- **`pull_request`** — Paseka **stops being the merger** for that trail. After the same Console/CLI diff review, approve **publishes**: push the trace branch to origin, then upsert a pull request via a **forge script**. The worktree stays until the PR is **merged on the forge**. Then runtime completes the final gate and removes the worktree. The clone’s default branch moves the way it already does after a remote merge (webhook sidecar / Console ff-only Pull).

Root proposals (R1) are unchanged.

Bees write meaning, not git hosting:

- existing `worktree.branch` → PR head
- existing `trace.title` → default PR title
- new operational `INSIGHT/pr.body` → PR description (markdown); last AFK work task is told to emit it

Runtime owns side effects: git push of the **worktree branch** (not default), then forge upsert. Hive Runtime does not consume honey for forge. The forge driver is **only** an external command (the same idea as `adapter: script`, not the same lifecycle): argv verb, JSON stdin/stdout, inherited process environment so `tea`/`gh` auth keeps working. First-party `tea` and `gh` examples are reference scripts of that contract, not privileged Go clients.

A colony without `origin` or without a configured forge command cannot silently fall back to local merge when policy is `pull_request`.

## User Stories

1. As a Beekeeper on a homelab apiary with protected `main`, I want isolated trails to land as pull requests, so that Gitea CI and branch protection see a reviewable head instead of a local merge I cannot push.
2. As a Beekeeper on a laptop-only clone, I want `local_merge` to remain the default, so that existing colonies do not start opening PRs.
3. As a Beekeeper, I want delivery policy on the colony (shareable), so that every apiary for this repo agrees how isolated work is published.
4. As a Beekeeper, I want forge credentials and the forge command on the machine (home config), so that tokens and `tea`/`gh` stay apiary-local like other adapter secrets.
5. As a Beekeeper, I want Queen Console Reviews to keep showing the accumulated worktree merge-diff before publish, so that I still review in Paseka ([017](./017-console-diff-review.md)).
6. As a Beekeeper, I want the isolated final-gate approve action, when delivery is `pull_request`, to publish the PR rather than run `worktree.Merge`, so that I do not get a local merge commit and a PR for the same trail.
7. As a Beekeeper, I do not want approve to merge on the forge in v1, so that CI and the Gitea/GitHub merge button stay the merger (same split idea as [023](./023-console-git.md): review success is not remote policy).
8. As a Beekeeper, I want publish to push only the trace worktree branch, so that leftover branches and default-branch history are not force-updated from Console.
9. As a Beekeeper, I want the first publish to be a normal fast-forward push of that branch, so that a missing remote ref is created without `--force`.
10. As a Beekeeper on Request changes rework, I want a later publish to update the **same** PR (same head) and allow `--force-with-lease` **only** on that worktree branch, so that rewritten commits can update the PR without rewriting `main`.
11. As a Beekeeper, I never want `--force` or `--force-with-lease` on the default branch from this flow, so that [023](./023-console-git.md) guarantees still hold for `main`.
12. As a Beekeeper, I want skip-hooks defaults on the branch push to match Console Git Push (skip unless opted in), so that homelab husky/`pre-push` does not block publish.
13. As a Beekeeper, I want a warning when `origin/<default>` is ahead of the merge base before publish, so that I fetch/rebase or accept a conflicted PR instead of being surprised (warn, do not auto-rebase in v1; do not block in v1).
14. As a Beekeeper, I want publish refused when there is no `origin`, so that a laptop-only repo with `pull_request` set fails closed instead of pretending to open a PR.
15. As a Beekeeper, I want publish refused when home `forge.command` is unset or not executable, so that policy cannot silently become local merge.
16. As a Beekeeper, I want git push failure and forge-script failure to stay separate error messages, so that I can tell credential-helper problems from `tea pr create` problems.
17. As a Beekeeper, I want the worktree and branch to remain after a successful publish, so that rework and `git push` still have a checkout.
18. As a Beekeeper, I want the final gate to stay open (`waiting_review`) after publish until the PR is merged on the forge, so that the trail is not “completed” while `main` still lacks the work.
19. As a Beekeeper, I want Queen Console to show the PR URL, number, and state on the review / trail surfaces, so that I can open Gitea without copying ids from logs.
20. As a Beekeeper, I want an idempotent publish (approve again / Update PR) to upsert title/body/draft on the existing PR for that head, so that I do not get a second PR after rework.
21. As a Beekeeper, I want Request changes ([017](./017-console-diff-review.md)) to keep working: gate stays `waiting_review`, rework runs on the same worktree, then publish updates the same PR.
22. As a Beekeeper, when the forge reports `merged`, I want Hive Runtime to complete the final gate and remove the worktree (same cleanup family as today’s merge), so that doctor/registry do not keep a live trail forever.
23. As a Beekeeper, I want that reconcile to run from Queen Console polling **and** from Hive Runtime without an open browser, so that an AFK homelab still closes the trail after I merge in Gitea.
24. As a Beekeeper, when the PR is closed without merge, I want the gate to stay open and a clear state shown, so that Paseka does not complete or locally merge as a fallback.
25. As a Beekeeper, I want inbound sync of default after a forge merge to stay the existing sidecar / Console ff-only Pull, so that this spec does not invent a second pull daemon.
26. As a Beekeeper, I do not want Console Git “Push default branch” to be required after a PR merge, so that homelab direction is: push **head**, pull **default**.
27. As a Beekeeper with two isolated trails at once, I want each to push its own branch without `checkout` of `main` on colony root, so that publish does not serialize on autostash/[009](./009-merge-autostash.md).
28. As a Beekeeper, I want root / hivewright R1 approve unchanged (no PR, no worktree merge), so that colony-config edits do not open product PRs.
29. As a Scout Bee, I want to keep emitting `worktree.branch`, so that the PR head is a conventional `feature/…` or `hotfix/…` name.
30. As a Builder Bee (or the last AFK work task), I want prompt guidance to emit `INSIGHT/pr.body`, so that the PR has a description without the Beekeeper writing it from scratch.
31. As a bee, I want `pr.body` to be last-write-wins operational metadata, so that a later pass can replace a weak first description.
32. As a downstream bee, I do not want `pr.body` injected into `{{.Insights}}`, so that markdown PR copy does not crowd prompt memory (same class as `trace.summary`).
33. As a colony author, I want no `dispatch: direct` subscribers on `pr.body`, so that describing a PR does not start another bee.
34. As a colony author, I want no completion-contract `required` and no runtime auto-synthesis of `pr.body` in v1, so that a missing body degrades (title + optional `trace.summary` fallback) instead of failing the builder run.
35. As a Beekeeper, I want HITL to override PR title and body on the publish form, so that I can fix prose without another bee run (same idea as `mergeMessage` vs `trace.summary`).
36. As a Beekeeper, I want default title from `trace.title` when I do not override, so that Console and the PR share a name.
37. As a Guard Bee, I want to keep emitting `VERIFICATION` and optional `review.note` only, so that I never open or merge PRs.
38. As a Beekeeper, I want the forge integration to be **any** executable that implements the script contract, so that Gitea (`tea`), GitHub (`gh`), or a custom host is a bash/python wrapper, not a Paseka release.
39. As a Beekeeper, I do not want bees configured with `adapter: tea` / a forge adapter, so that honey, run directories, and auto-`code.proposal` never attach to hosting CLI.
40. As a Beekeeper, I want the forge child to inherit the Console/runtime environment and non-TTY stdin (JSON only), so that `tea`/`gh` helpers work like `docker exec` git and cannot hang on a username prompt.
41. As a Beekeeper, I want forge stdout to be a single JSON object, so that logs on stdout cannot be mistaken for a PR URL.
42. As a Beekeeper, I want `get` with no PR to return `found: false` and exit 0, so that scripts using `set -e` do not treat “no PR yet” as a hard failure.
43. As a Beekeeper, I want a `capabilities` verb, so that Console can hide a future Merge-on-forge control when the script does not implement it.
44. As a Beekeeper, I want v1 scripts to implement `upsert` and `get` only, so that a Gitea wrapper can ship without merge API.
45. As a Beekeeper, I want upsert to be keyed by **head branch**, so that one trace / one worktree / one PR stays the identity (not a number the bee invents).
46. As a platform contributor, I want the Go side to only exec, validate JSON, time-bound the child, and apply policy, so that adding GitLab is a new script, not a new module.
47. As a Beekeeper, I want example `tea` and `gh` scripts in the repository as the contract reference, so that I copy rather than reverse-engineer.
48. As a Beekeeper, I want forge failures to include stderr (capped) in Console/CLI, so that a missing `tea` login is diagnosable.
49. As a Beekeeper, I want `system.kill` to leave an already-opened PR open on the forge, so that Paseka does not close someone else’s review as a side effect; I close or merge it on the host.
50. As a Medic Bee / `paseka doctor` path, I want worktree cleanup still keyed by `traceId`, so that a published-but-unmerged trail is visible as a live worktree, not an orphan.
51. As a Beekeeper, I want leftover local branch cleanup after a forge merge to reuse [023](./023-console-git.md) safe delete (merged, no worktree), so that this spec does not invent `branch -D`.
52. As a Beekeeper using Queen Shell, I want `paseka proposal approve` to follow the same delivery policy as Console, so that CLI and Reviews cannot diverge (merge vs publish).
53. As a Beekeeper on Telegram, I want a notification with the PR URL after publish, so that I can merge from the phone in Gitea; forge merge from Telegram is out of scope for v1.
54. As a Beekeeper, I want no new domain bus kinds for `pr.opened` / `git.pushed` in v1, so that the task ledger does not record replica hosting (same as [023](./023-console-git.md)).
55. As a Beekeeper, I want an empty isolated merge candidate to still skip the final gate ([task ledger](../reference/task-ledger.md)), so that scout-only trails do not open hollow PRs.
56. As a Beekeeper, I want `local_merge` approve to keep using [009](./009-merge-autostash.md) autostash, so that opting into PRs does not regress laptop merge.
57. As an implementer, I want protocol validation to reject empty and oversized `pr.body`, so that the forge child is not fed unbounded stdin.
58. As a Beekeeper, I want a `protocolVersion` on forge JSON, so that a later `checks` field does not silently break old scripts.
59. As a Beekeeper, I want draft PRs to be optional on the publish form (default ready), so that I can open a draft when CI should not merge yet.
60. As a colony author, I want `paseka doctor` / load to reject unknown `defaults.delivery` values, so that a typo cannot mean local merge.

## Implementation Decisions

### 1. Product split

- Isolated path only. Root R1 ([008](./008-code-proposal-workspaces.md)) never publishes a PR and never calls the forge script.
- Default delivery remains **`local_merge`**. `pull_request` is opt-in on the colony manifest (`defaults.delivery`).
- When `defaults.delivery` is `pull_request`, final-gate approve **must not** call `worktree.Merge`. The two mergers must not run for the same trail.
- [023](./023-console-git.md) still operates the clone vs origin for the **default** branch. This spec adds push of the **trace head** as part of review publish, not as a substitute for 023 Push of `main`.
- Queen Shell, Queen Console, and Telegram proposal notify share one review/publish implementation (same family as today’s `review.Approve`).

### 2. Colony vs home

- **Colony (shareable):** `defaults.delivery`: `local_merge` | `pull_request`. Unknown values fail load/doctor.
- **Home (apiary):** forge executable argv (`forge.command`), same process user as `paseka console` / `paseka run`.
- Do not put forge tokens in `.paseka/` or in bee YAML. Scripts read `tea` / `gh` / host config themselves. Do not pass tokens on argv.
- **Gap (not locked):** per-cue or per-trace delivery override. v1 is colony-wide.

### 3. Bee contract: `INSIGHT/pr.body`

- Operational last-write-wins kind, same class as `trace.title` / `trace.summary` / `worktree.branch`: not projected into `{{.Insights}}`, no routing, ledger ignores the event (no task-queue `changed` from this kind alone).
- Payload: `{ "kind": "pr.body", "body": "<markdown>" }` only.
- Bee language: **pull request body**; wire kind **`pr.body`**.
- Validation: required after trim; reject empty/whitespace; maximum **8000** characters after trim (longer than `trace.summary`’s 800; still bounded for stdin).
- Last AFK work task (`IsLastWorkTask`) prompt must-emit, prompt-level only (not completion-contract, not runtime synthesis).
- Resolve for publish: latest valid `pr.body`; else `trace.summary`; else empty. HITL override on the publish form wins when non-empty (same split idea as [012](./012-trace-summary.md): first-line title override vs body).
- Default **title**: HITL override if set; else resolved `trace.title`; else existing merge-message-style fallback using `traceId` (must remain valid as a PR title, not necessarily a git subject).
- Runtime does **not** auto-emit `pr.body`. Manual `paseka event emit` remains valid.
- Do not add `pr.title` in v1 (`trace.title` is enough).

### 4. Git (not forge)

- Publish sequence: (1) push `refs/heads/<resolvedBranch>` to `origin`; (2) forge `upsert`. Never fold push into the script.
- Push from colony git, without checking out the default branch on colony root (isolated worktrees must not require [009](./009-merge-autostash.md)).
- Resolved branch: same helper as merge-diff ([020](./020-worktree-branch.md)): live worktree HEAD, else registry, else insight, else `paseka/<traceId>`.
- First push: no force. Subsequent pushes when the remote head exists and is not a descendant: `--force-with-lease` on **that branch only**.
- Refuse push of default / `HEAD` / `main`/`master` as a worktree branch (already rejected at `worktree.branch` validate).
- Skip hooks by default on this push (`--no-verify` + `HUSKY=0` when skipping), same operator story as [023](./023-console-git.md). Opt-in `runHooks` may share the Console Git mechanism or a publish-form flag — **gap:** one mechanism, not two hidden defaults.
- Origin-ahead vs default: **warn** on the publish form using already-fetched remote-tracking refs (no auto-fetch in the Reviews poll loop). Auto-rebase of the worktree onto `origin/<default>` is out of scope.
- Dirty colony **root** does not block head push (push is not a merge into root). A dirty **worktree** is the bee’s problem; v1 does not auto-commit uncommitted worktree files (same as today: merge-diff is committed history vs default).

### 5. Final gate and ledger

- Activation of `review: final` / `_review` is unchanged (`AllAFKTasksCompleted` + isolated merge candidate).
- **`local_merge`:** approve remains merge + `task.completed` + worktree remove ([009](./009-merge-autostash.md)).
- **`pull_request`:** approve = publish (push + upsert). On success, keep the task `waiting_review`; record PR identity in **machine-local** state (URL, number, head, last `state` from `get`). Do not emit `task.completed` at publish.
- Reconcile: `get` by head; when `state` is `merged`, emit `task.completed` (summary may include PR URL/SHA if the script returned it) and run today’s worktree remove + registry unregister. Do not `checkout` default; do not create a local merge commit.
- When `state` is `closed` (not merged): do not complete; surface state; Beekeeper republishes, reopens on the forge, or rejects/kills the trail.
- Request changes: unchanged rework planning; publish after rework is upsert + force-lease push as needed.
- `system.kill`: cancel tasks as today; do not call forge close; worktree cleanup follows existing kill/purge rules (**gap:** whether kill should `worktree remove` while a PR is still open — prefer existing kill behavior; note in doctor that the remote branch/PR may remain).
- No new task status in v1 (`waiting_review` covers “reviewed, PR open”). **Gap if UX is too muddy:** a dedicated status can be a follow-up; do not invent it in the first implementation unless Reviews copy cannot distinguish “not published yet” vs “PR open” using machine-local PR identity.

### 6. Forge script contract

Not a bee adapter: no honey, no `.paseka/runs/<traceId>/<agentId>/`, no auto mutation from `git diff`, no `paseka event emit` from the script.

Home `forge.command` is an argv prefix. Runtime invokes:

```text
<command...> <op>
```

with cwd = colony root, stdin = one JSON request, stdout = one JSON object, stderr = diagnostics, env = parent `Environ()` plus small `PASEKA_FORGE_*` hints (`OP`, `COLONY_ROOT`, `TRACE_ID`, `HEAD`, `BASE`). **PR body is only on stdin**, never argv.

**v1 ops:** `capabilities`, `upsert`, `get`. **`merge` is out of scope** unless `capabilities` lists it; Console does not send `merge` in v1 even if listed.

Exit codes: `0` if stdout JSON is the success document (including `get` with `found: false`); any other exit is failure (stderr to operator). Do not use a special exit for “not found.”

The following shapes are the locked IPC (prototype of the contract, not a product demo). Request always includes `protocolVersion` (v1 = `1`) and `op`.

`capabilities` response:

```json
{ "protocolVersion": 1, "ops": ["upsert", "get"] }
```

`upsert` / `get` request (body omitted on `get` if unused):

```json
{
  "protocolVersion": 1,
  "op": "upsert",
  "head": "feature/live-bees-header",
  "base": "main",
  "title": "Live bees in Queen Console header",
  "body": "## Why\n…",
  "draft": false,
  "traceId": "trace-019f…",
  "origin": "https://gitea.example/org/repo.git"
}
```

Success / `get` response:

```json
{
  "protocolVersion": 1,
  "found": true,
  "number": 42,
  "url": "https://gitea.example/org/repo/pulls/42",
  "head": "feature/live-bees-header",
  "base": "main",
  "state": "open",
  "draft": false
}
```

`state` enum v1: `open` | `merged` | `closed`. `get` with no PR: `{ "protocolVersion": 1, "found": false }` and exit 0.

Runtime fail closed when: stdout is not a single JSON object, `protocolVersion` missing/unsupported, required fields missing on `found: true` (`url`, `state`, `head`), or `upsert` returns `found: false`.

Timeout the child. Do not attach a TTY. Empty stdin besides the JSON document.

**Upsert semantics (script responsibility):** find PR by **head**; if none, create; if one, update title/body/draft. Multiple PRs for one head: fail closed with stderr (do not pick arbitrarily).

Example scripts for Gitea (`tea`) and GitHub (`gh`) ship in the repo as contract references. Runtime does not special-case those binaries.

**Gap (not locked):** auto-detect forge from `origin` host vs requiring explicit `forge.command`. v1 requires explicit command when `delivery` is `pull_request`.

### 7. Console / CLI / Telegram

- Reviews final-gate: keep merge-diff preview. When `pull_request`, primary action is publish (Open / Update PR), not local merge. Show origin-ahead warning (reuse [023](./023-console-git.md) hint). Show resolved title/body preview; HITL can edit title/body; optional draft checkbox.
- After publish: show URL/state; keep Request changes; keep reject-unstructured as today (does not close the remote PR in v1).
- Trace detail / Git tab: PR URL when machine-local identity exists (Git tab may list it on the worktree row). **Gap:** plaque copy for “PR open” vs git ahead/behind — keep git plaque about clone↔origin; put PR on Reviews/trace.
- Queen Shell: `paseka proposal approve` honors delivery; flags for title/body/draft overlay as needed (names **gap:** do not collide with `--merge-message` / `--summary`; prefer explicit `--pr-title` / `--pr-body` or reuse merge message as title-only — pick one in implementation without a second silent mapping).
- Telegram: include PR URL in the final-gate / publish success card when known. No forge merge command in v1.

### 8. Modules (durable, no paths)

- Protocol + insight-kinds reference: `pr.body`.
- Prompt partials: last-work-task emit guidance.
- Colony load/doctor: `defaults.delivery`.
- Home config: `forge.command`.
- Git helpers beside existing worktree/gitroot: push head, force-with-lease rules.
- Forge runner: exec + JSON validate + capabilities cache per process is optional.
- Review approve: branch on delivery; stash/merge path only for `local_merge`.
- Hive Runtime: reconcile merged PRs (coarse interval or on existing reactor ticks — implementation pick; must work with Console stopped).
- Machine-local state: PR identity keyed by `traceId` (worktree registry extension or sibling map; not the task ledger KV).
- Console HTTP/SPA: publish action, PR fields, no NATS required for `get` refresh (same independence idea as [023](./023-console-git.md) / [022](./022-console-system-info.md)).
- No `adapter:` name for forge. No colony.yaml git remote editor.

## Testing Decisions

Good tests assert **observable git, process, ledger, and HTTP behavior**, not internal function names. Forge tests use a **fixture executable** that reads stdin JSON and writes stdout JSON (no network, no `tea`/`gh`). Git tests use local `git init` / `--bare` remotes (prior art: worktree merge/diff tests, planned [023](./023-console-git.md) fixtures).

Cover at least:

- Protocol: accept valid `pr.body`; reject empty, too long; ledger apply does not add tasks; kind excluded from `{{.Insights}}` projection tests if those exist for `trace.summary`.
- `IsLastWorkTask` prompt contains PR-body guidance when the flag is true (prior art: `trace.summary` prompt tests).
- Colony load: default `local_merge`; `pull_request` accepted; unknown delivery errors.
- `local_merge` approve still merges and completes (prior art: review/worktree merge tests); `pull_request` approve does not create a merge commit on default and does not remove the worktree.
- Push: creates remote branch; second push with rewritten head uses lease; never force-pushes default.
- Publish without origin or without `forge.command` errors; does not merge locally.
- Fixture `upsert` then `get`; second upsert same head does not require a second create (script fixture can assert call log).
- `get` `found: false` + exit 0 is success; garbage stdout is failure.
- Reconcile: fixture `state: merged` → `task.completed` + worktree gone; `closed` → still `waiting_review`.
- Request changes then publish still one PR identity (same head in fixture).
- Root R1 approve still no forge spawn.
- Console/SPA: publish control and PR URL fields exist when policy is PR (static string tests, prior art: [023](./023-console-git.md) tab/API strings).
- Honey: forge spawn does not decrement energy.

Do not require Gitea, GitHub, or `tea`/`gh` in unit tests. Optional later: example-script smoke against `tea` in a manual homelab checklist, not CI.

## Out of Scope

- Auto-merge on the forge; Console/CLI `merge` op; waiting on CI checks beyond what `get` may later add.
- Auto-rebase / merge `origin/<default>` into the worktree before push.
- Pushing the worktree branch as a substitute for local merge **and** still running `worktree.Merge` (dual merger).
- Changing `local_merge` to push default on approve ([023](./023-console-git.md) still forbids auto-push of `main`).
- Bee adapters named `gitea`/`github`/`tea`; honey-consuming forge runs; `paseka event emit` from forge scripts.
- In-process Gitea/GitHub SDKs or OAuth; storing `GITEA_TOKEN` as a Paseka-native git auth path ([023](./023-console-git.md)).
- Closing or commenting on PRs from reject, Request changes, or `system.kill`.
- Per-task branches or one PR per `taskId` ([020](./020-worktree-branch.md)).
- `INSIGHT/pr.title`, labels, reviewers, assignees, issue links.
- New bus kinds `pr.opened`, `git.pushed`, `pr.merged`.
- Queen Shell `paseka git` / `paseka pr`.
- Telegram forge merge/approve-on-host.
- Webhook from Gitea into Paseka for PR events (reconcile is pull/`get`).
- Multi-remote; fork PRs; stacked PRs.
- Windows-first bash; the contract is argv (Python/Go binaries are valid).
- Editable `pr.body` as a required completion contract.
- Root-path auto-commit (R2) via PR.
- Blocking publish when origin is ahead (warn only, same spirit as [023](./023-console-git.md) warn-on-approve).

## Further Notes

- **Why not merge locally and also open a PR:** two histories. Homelab protected `main` already rejects the 023 Push-default story; the PR host must be the merger.
- **Why completed only after forge merge:** publish is network + hosting. Same failure-domain split as [023](./023-console-git.md) (merge vs Push) and [009](./009-merge-autostash.md) (merge vs stash restore). Completing on open would mark the Flight Trail done while default is unchanged.
- **Why a script, not `adapter: script`:** bee script runs are AFK dispatch with run dirs, honey, and optional auto-proposal. Hosting is a privileged operator side effect, like `git push`.
- **Why stdin JSON:** PR bodies contain quotes and markdown; argv and env are the wrong place.
- **Thin slice if implementation is staged:** `pr.body` + head push + upsert can land before reconcile-on-merged, but shipping publish-without-reconcile must not emit `task.completed` or operators will think the trail is done. Do not ship “complete on open” as a silent default.
- Related: [008](./008-code-proposal-workspaces.md), [009](./009-merge-autostash.md), [012](./012-trace-summary.md), [017](./017-console-diff-review.md), [020](./020-worktree-branch.md), [023](./023-console-git.md), [insight kinds](../reference/insight-kinds.md), [task ledger](../reference/task-ledger.md), [homelab deployment](../guide/homelab-deployment.md).
