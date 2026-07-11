# Paseka documentation

Index of the main docs. Feature specs under `specs/` are not listed here.

| Document | Description |
| -------- | ----------- |
| [001-brief.md](001-brief.md) | Product brief: choreographed AI swarm concept, EDA contracts, `traceId` / `energyToken`, stack (NATS + JetStream), HITL, and MVP next steps |
| [002-paseka-glossary.md](002-paseka-glossary.md) | Bee glossary: branding, user-facing terms, agent roles, and domain model vocabulary |
| [003-architecture.md](003-architecture.md) | Colony architecture: `.paseka/` and machine-local config, `paseka init`, adapters, worktree flow, package layout |
| [004-prompt-templates.md](004-prompt-templates.md) | Prompt templates in `.paseka/prompts/`: `text/template`, partials, context variables, and render-at-dispatch |
| [005-task-ledger.md](005-task-ledger.md) | Task Ledger: `traceId` → `taskId` → `agentId`, task lifecycle, and projected trace state |
| [006-interactive-sessions.md](006-interactive-sessions.md) | Interactive HITL sessions: `bee chat`, SessionAdapter + PTY, session registry, and Ghostty attach |
| [007-cli.md](007-cli.md) | Queen Shell (`paseka`) reference: init/run/status, bee run/chat, energy, and other CLI commands |
| [008-bee-routing.md](008-bee-routing.md) | Bee routing: `subscribes` / `publishes`, Reactor, task vs direct dispatch |
| [009-insight-kinds.md](009-insight-kinds.md) | `INSIGHT` taxonomy: difference from `VERIFICATION`, payload kinds, and projection into `{{.Insights}}` |
| [010-bee-config.md](010-bee-config.md) | Bee role YAML (`.paseka/bees/<role>.yaml`): schema, adapters, `command` / `post_exec`, params, contracts |
| [999-backlog.md](999-backlog.md) | Deferred ideas and follow-ups outside the current MVP |

Russian index: [README-RU.md](README-RU.md).

Agent index: [llms.txt](llms.txt) (full corpus: [llms-full.txt](llms-full.txt)).
