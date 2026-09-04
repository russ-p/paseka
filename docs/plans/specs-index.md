# Specs index

Feature design specs live in [`docs/specs/`](../specs/) in the git repository. They are **not** published on the documentation site (MkDocs excludes them). Use this table as a map; open files on GitHub or locally in the repo.

After a spec ships, prefer a [Changelog](changelog.md) entry plus updates to guide/reference docs. Specs remain the design record.

| Spec | Status | Summary |
| ---- | ------ | ------- |
| [001-pi-integration](../specs/001-pi-integration.md) | Implemented | Pi CLI as first-class AFK + interactive adapter |
| [002-queen-console-mvp](../specs/002-queen-console-mvp.md) | Implemented (MVP baseline) | Queen Console SPA + API surface |
| [003-hive-evals](../specs/003-hive-evals.md) | In progress (Phase 2 largely done; Tier C/4 open) | Eval harness / eval colony |
| [004-live-bees-indicator](../specs/004-live-bees-indicator.md) | Implemented | Live agent processes in Console header |
| [005-feature-ideation-flow](../specs/005-feature-ideation-flow.md) | Draft (colony reference) | Classify → grill → `spec.ready` → breakdown |
| [006-human-gateway-invites](../specs/006-human-gateway-invites.md) | Implemented | Session invites, accept/reject, `done_when` |
| [007-colony-eda-topology](../specs/007-colony-eda-topology.md) | Implemented | Config-derived EDA graph (Console + CLI) |
| [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md) | Implemented | Isolated vs root code proposals |
| [009-merge-autostash](../specs/009-merge-autostash.md) | Implemented | Autostash dirty root on merge approve |
| [010-telegram-human-gateway](../specs/010-telegram-human-gateway.md) | Implemented (MVP) | Telegram as async Human Gateway (notify + status/task/energy/invites/proposals) |
| [011-trace-title](../specs/011-trace-title.md) | Implemented | `INSIGHT/trace.title` for Flight Trail display names |
| [012-trace-summary](../specs/012-trace-summary.md) | Implemented | `INSIGHT/trace.summary` for Console subtitle + merge body |
| [013-system-kill](../specs/013-system-kill.md) | Implemented | Hard `SIGNAL/system.kill` + `paseka kill` |
| [014-artifacts-protocol](../specs/014-artifacts-protocol.md) | Implemented | Trace comb + `SIGNAL/artifact.written`; bee scan-flush; Console list/preview; export `--include artifacts`; human producer helper |
| [015-deferred-event-emit](../specs/015-deferred-event-emit.md) | Implemented | General deferred `event emit` buffer (`--defer`, flush on success; 014 does not depend on it) |
| [016-cue-layer](../specs/016-cue-layer.md) | Implemented | Named Forage Cue ingress (`.paseka/cues/`); CLI + Console + Telegram `cue:`; optional per-cue `energy_budget` |
| [017-console-diff-review](../specs/017-console-diff-review.md) | Implemented | Queen Console merge-diff viewer, annotated review comments, final-gate Request changes rework |
| [018-cli-colony-status](../specs/018-cli-colony-status.md) | Implemented | `paseka status` read-only colony snapshot (CLI alternative to Console dashboard; energy in MVP; `--check` for substrate probe) |
| [019-model-aliases](../specs/019-model-aliases.md) | Implemented | Colony `model_aliases` map; home overlay; resolves `params.model` at dispatch |
| [020-worktree-branch](../specs/020-worktree-branch.md) | Implemented | `INSIGHT/worktree.branch` to name/rename the isolated trace worktree git ref |
| [021-provider-session-logs-export](../specs/021-provider-session-logs-export.md) | Draft | AFK + HITL Cursor/Pi persist `providerSessionId`; export `--include agent-logs`; Cursor jsonl reader; remaining: Pi reader, `store.db`, `events.ndjson` cleanup |
| [022-console-system-info](../specs/022-console-system-info.md) | Implemented | Queen Console Host plaque + System tab: CPU/RAM, cheap identity, top processes (console host view, no Docker APIs) |
| [023-console-git](../specs/023-console-git.md) | Implemented | Queen Console Git tab: clone vs origin, explicit fetch/push/ff-only pull, worktrees, leftover branches (homelab; no auto-push on approve) |
| [024-pull-request-delivery](../specs/024-pull-request-delivery.md) | Draft | Isolated trails: opt-in PR publish instead of local merge; bees `pr.body`; forge via external CLI script contract |
| [025-cursor-session-resume](../specs/025-cursor-session-resume.md) | Draft | Console/CLI resume of a finished Cursor HITL chat via stored `providerSessionId`; new Paseka session, same UUID; no Pi/Claude/AFK |
| [026-opencode-adapter](../specs/026-opencode-adapter.md) | Draft | OpenCode CLI as first-class AFK + interactive adapter; `paseka init --adapter opencode` |
| [027-config-profiles](../specs/027-config-profiles.md) | Draft | Named colony + home settings overlays; `paseka --profile`; fail-closed; global `adapter:` replaces LLM bees |

Deferred ideas (not specs): [Backlog](backlog.md).
