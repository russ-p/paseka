## Choreographic coding agents runtime

Paseka is a decentralized, event-driven AI agent swarm for solo developers. Agents communicate through NATS/JetStream using choreographed message contracts (`SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION`) rather than a central orchestrator.

## Tech stack

- **Language:** Go — platform, CLI, runtime, and Queen Console API.
- **Message bus:** NATS + JetStream (event sourcing, KV, Object Store).
- **CLI:** Queen Shell (`cmd/paseka`) — single binary for init, run, status, and minimal hive management.
- **Web UI:** Queen Console — embedded SPA + Go HTTP/WebSocket API via `paseka console`; operator guide in [docs/guide/queen-console.md](docs/guide/queen-console.md).
- **Agents (Bees):** implemented in Go by default. Optional Python workers may subscribe to the same NATS contracts later for code/AST-heavy tasks.

## Colony model

Work centers on a **git repository**. Configuration is split:

- **Project:** `.paseka/` in the repo — shareable colony manifest, bee definitions, gitignored worktrees.
- **Machine-local:** `~/.config/paseka/<project-slug>/` — secrets, adapter credentials, runtime state.

`paseka init` bootstraps both. See [docs/guide/colony-layout.md](docs/guide/colony-layout.md). Isolated worktrees live under `.paseka/worktrees/<traceId>/`; git branch is `INSIGHT/worktree.branch` when set, else `paseka/<traceId>`.

## Adapters

Bees do not embed LLM logic. An **adapter** launches external agents (Cursor Agent CLI `agent`, Pi, Claude Code, OpenCode, or a script); each invocation gets `.paseka/runs/<traceId>/<agentId>/` for file IPC (`prompt.txt`, `summary.md`, `meta.json`, `status.json`). AFK runs use `Adapter.Run()` (`paseka bee run`); interactive HITL sessions use `SessionAdapter` + PTY (`paseka bee chat`) — see [docs/guide/interactive-sessions.md](docs/guide/interactive-sessions.md). Worktrees under `.paseka/worktrees/<traceId>/` isolate code mutations until review.

## Prompt templates

Prompt templates live in `.paseka/prompts/` (committed); each bee references a template in `bees/*.yaml`. Runtime renders with `text/template` via `internal/prompts`, then `internal/runtime.Dispatcher` passes the result to the adapter. See [docs/guide/prompt-templates.md](docs/guide/prompt-templates.md). Bee YAML schema: [docs/guide/bee-config.md](docs/guide/bee-config.md).

## Documentation

Read these before making architectural or naming decisions:

- [docs/idea/principles.md](docs/idea/principles.md) — Choreography model, contracts, honey, HITL.
- [docs/idea/glossary.md](docs/idea/glossary.md) — Bee glossary: branding, user-facing terms, agent roles, and domain vocabulary.
- [docs/guide/colony-layout.md](docs/guide/colony-layout.md) — Colony config layout and `paseka init`.
- [docs/guide/cli.md](docs/guide/cli.md) — Queen Shell commands, first run, and troubleshooting.
- [docs/reference/event-contracts.md](docs/reference/event-contracts.md) — Domain event kinds and authoring rules.
- [docs/architecture/overview.md](docs/architecture/overview.md) — Adapters, worktrees, package layout.
- [docs/guide/interactive-sessions.md](docs/guide/interactive-sessions.md) — Interactive agent sessions (HITL), `bee chat`, session registry, Ghostty attach.
- [docs/guide/bee-config.md](docs/guide/bee-config.md) — Bee role YAML: adapter, params, command/post_exec, routing and completion fields.
- [docs/guide/homelab-deployment.md](docs/guide/homelab-deployment.md) — Server/apiary container + Queen Console.

## After Go changes

After editing Go code, run formatting, lint, and a full build before finishing:

```bash
gofmt -w .
golangci-lint run
go build -o paseka ./cmd/paseka
```

Config: [`.golangci.yml`](.golangci.yml) (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`). Fix compilation errors from `go build` and issues reported by `golangci-lint`.
