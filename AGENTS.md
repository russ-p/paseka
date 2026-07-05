## Choreographic coding agents runtime

Paseka is a decentralized, event-driven AI agent swarm for solo developers. Agents communicate through NATS/JetStream using choreographed message contracts (`SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION`) rather than a central orchestrator.

## Tech stack

- **Language:** Go — platform, CLI, runtime, and future Queen Console API.
- **Message bus:** NATS + JetStream (event sourcing, KV, Object Store).
- **CLI:** Queen Shell (`cmd/paseka`) — single binary for init, run, status, and minimal hive management.
- **Web UI (later):** Queen Console — SPA frontend + Go HTTP/WebSocket API; not part of MVP.
- **Agents (Bees):** implemented in Go by default. Optional Python workers may subscribe to the same NATS contracts later for code/AST-heavy tasks.

## Colony model

Work centers on a **git repository**. Configuration is split:

- **Project:** `.paseka/` in the repo — shareable colony manifest, bee definitions, gitignored worktrees.
- **Machine-local:** `~/.config/paseka/<project-slug>/` — secrets, adapter credentials, runtime state.

`paseka init` bootstraps both. See [docs/003-architecture.md](docs/003-architecture.md).

## Adapters

Bees do not embed LLM logic. An **adapter** launches external agents via **Cursor Agent CLI** (`agent`); each invocation gets `.paseka/runs/<traceId>/<agentId>/` for file IPC (`prompt.txt`, `result.txt`, `meta.json`, `status.json`). AFK runs use `Adapter.Run()` (`paseka bee run`); interactive HITL sessions use `SessionAdapter` + PTY (`paseka bee chat`) — see [docs/006-interactive-sessions.md](docs/006-interactive-sessions.md). Worktrees under `.paseka/worktrees/<traceId>/` isolate code mutations until review.

## Prompt templates

Prompt templates live in `.paseka/prompts/` (committed); each bee references a template in `bees/*.yaml`. Runtime renders with `text/template` via `internal/prompts`, then `internal/runtime.Dispatcher` passes the result to the adapter. See [docs/003-architecture.md](docs/003-architecture.md) §2.1.

## Documentation

Read these before making architectural or naming decisions:

- [docs/001-brief.md](docs/001-brief.md) — Product brief: core concepts, architecture (EDA, traceId, energyToken), tech stack (NATS + JetStream), human-in-the-loop gateway, and MVP next steps.
- [docs/002-paseka-glossary.md](docs/002-paseka-glossary.md) — Bee glossary: branding, user-facing terms, agent roles, architecture metaphors, and domain model vocabulary (technical ↔ bee language).
- [docs/003-architecture.md](docs/003-architecture.md) — Colony config layout, `paseka init`, adapter contract, worktree flow, package layout.
- [docs/006-interactive-sessions.md](docs/006-interactive-sessions.md) — Interactive agent sessions (HITL), `bee chat`, session registry, Ghostty attach.

## After Go changes

After editing Go code, run formatting and a full build before finishing:

```bash
gofmt -w .
go build -o paseka ./cmd/paseka
```

Fix any compilation errors reported by `go build`.
