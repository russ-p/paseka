# Specs index

Feature design specs live in [`docs/specs/`](../specs/) in the git repository. They are **not** published on the documentation site (MkDocs excludes them). Use this table as a map; open files on GitHub or locally in the repo.

After a spec ships, prefer a [Changelog](changelog.md) entry plus updates to guide/reference docs. Specs remain the design record.

| Spec | Status | Summary |
| ---- | ------ | ------- |
| [001-pi-integration](../specs/001-pi-integration.md) | Implemented | Pi CLI as first-class AFK + interactive adapter |
| [002-queen-console-mvp](../specs/002-queen-console-mvp.md) | Implemented (MVP baseline) | Queen Console SPA + API surface |
| [003-hive-evals](../specs/003-hive-evals.md) | In progress | Eval harness / eval colony |
| [004-live-bees-indicator](../specs/004-live-bees-indicator.md) | Implemented | Live agent processes in Console header |
| [005-feature-ideation-flow](../specs/005-feature-ideation-flow.md) | Draft (colony reference) | Classify → grill → `spec.ready` → breakdown |
| [006-human-gateway-invites](../specs/006-human-gateway-invites.md) | Implemented | Session invites, accept/reject, `done_when` |
| [007-colony-eda-topology](../specs/007-colony-eda-topology.md) | Implemented | Config-derived EDA graph (Console + CLI) |
| [008-code-proposal-workspaces](../specs/008-code-proposal-workspaces.md) | Implemented | Isolated vs root code proposals |
| [009-merge-autostash](../specs/009-merge-autostash.md) | Implemented | Autostash dirty root on merge approve |

Deferred ideas (not specs): [Backlog](backlog.md).
