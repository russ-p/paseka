# Colony layout and configuration

Start here for where colony config lives, how `.paseka/` relates to machine-local state, and what `paseka init` creates.

For adapters, run directories, worktrees, and package layout see [Architecture overview](../architecture/overview.md).

## 1. Colony-centric model

| Concept | Location | Role |
| ------- | -------- | ---- |
| **Colony** | Git repo root | Source of truth for code, history, and shareable hive config |
| **Apiary** | Developer machine | Hosts Hive Runtime, NATS, and local adapter credentials |
| **Bee** | Config + runtime | A role (Scout, Guard, Builder…) bound to an **adapter** that drives an external agent |

The runtime never owns LLM logic. It **orchestrates** external tools via **adapters** — the **Cursor Agent CLI** (`agent`), the **Pi CLI** (`pi`), **Claude Code**, and **script** commands — reads their output, and publishes results to the NATS bus as contract events.

---

## 2. Two-tier configuration

### Project-local: `.paseka/` (in repo)

Version-controlled colony definition. Safe to commit; no secrets.

```
.paseka/
├── colony.yaml          # colony manifest: bees, routes, defaults
├── bees/                # per-bee adapter bindings and non-secret params
│   ├── scout.yaml
│   └── builder.yaml
├── prompts/             # prompt templates (committed); see §2.1
│   ├── _partials/       # shared snippets (JSON contract, tone, etc.)
│   ├── scout.md
│   └── builder.md
├── .gitignore           # ignores worktrees/, runs/, cache/, *.local.yaml
├── runs/                # gitignored — per-agent file IPC (architecture overview)
│   └── <traceId>/
│       ├── <agentId>/
│       │   ├── prompt.txt
│       │   ├── system.txt   # optional — rendered system_template
│       │   ├── result.txt
│       │   ├── meta.json
│       │   └── status.json
│       └── tasks/
│           └── <taskId>/
│               ├── task.md
│               └── runs.ndjson
└── worktrees/           # gitignored — isolated mutation workspaces
    └── <traceId>/
```

**`colony.yaml`** — colony identity, default branch, bee registry, optional **sectors** (module/subfolder workspace scopes), NATS subject prefixes (optional overrides), colony-wide defaults including per-trace honey reserve (`defaults.energy_budget`, default `12`), and optional **`auto_invites`** (HITL choreography that publishes `session.invite` when bus events match — see [bee routing](../reference/bee-routing.md)).

```yaml
defaults:
  prompt_template: default.md
  system_template: default-system.md   # optional colony-wide role context
  energy_budget: 12
```

Each `traceId` shares one **Honey Reserve** (`energyToken`): every adapter dispatch (`task.ready` and direct routing) consumes one token. When the reserve is empty, tasks move to `blocked` with summary `Honey reserve exhausted`. Beekeepers can top up via `paseka energy add --trace <id> --amount <n>`.

Example sectors for monorepos or git-submodule layouts:

```yaml
sectors:
  frontend:
    path: frontend
  backend-users:
    path: backend/users
```

A **sector** is a named path inside the colony. Tasks may optionally set `sector`; bees may declare a default `sector` in `bees/*.yaml`. Runtime resolves the adapter workspace as `colonyRoot/<sector.path>` or `.paseka/worktrees/<traceId>/<sector.path>` when `worktree: true`. The colony root remains the audit boundary for `.paseka/runs/`.

**`bees/*.yaml`** — one file per role: binds the bee to an adapter, prompt template(s), optional `command` / `post_exec`, sector/worktree, and routing rules. Full schema, examples, and variable substitution: [bee config](bee-config.md). Event routing (`subscribes` / `publishes`): [bee routing](../reference/bee-routing.md). Project-local overrides that must not be committed live in `*.local.yaml` (gitignored).

### 2.1 Prompt templates

Templates live in **`.paseka/prompts/`** — version-controlled, one colony, shareable across machines. Each bee may reference one or two templates from its `bees/<role>.yaml`:

| Field | Artifact | Role |
| ----- | -------- | ---- |
| `system_template` (optional) | `system.txt` | Standing role context — injected by the adapter, not the first chat turn |
| `prompt_template` | `prompt.txt` | User/task turn for AFK runs; optional kickoff for interactive chat |

When `system_template` is unset, behavior matches the previous single-template model (full prompt as positional argv only). Full variable list, partials, and override precedence: [prompt templates](prompt-templates.md). Bee YAML schema: [bee config](bee-config.md).

**Bee config → templates:**

```yaml
# .paseka/bees/builder.yaml
role: builder
adapter: cursor
system_template: builder-system.md   # optional — role / standing instructions
prompt_template: builder.md          # user/task turn
worktree: true
```

**Rendering:** Go `text/template` at dispatch time. Runtime builds a **PromptContext** from bus event + colony state, writes `prompt.txt` (and `system.txt` when `system_template` is set) under `.paseka/runs/<traceId>/<agentId>/`, then passes rendered strings to the adapter.

Colony-wide fallbacks when a bee omits a field: `defaults.prompt_template` and optional `defaults.system_template` in `colony.yaml`.

Do **not** store prompts in `~/.config/paseka/` — they belong to the colony and should ride with the repo. Home config only holds secrets and runtime state.

**Bee Language vs technical:** UI/docs may say «Scout Bee»; templates can use bee tone for HITL readability. Bus payloads and JSON partials stay technical (`SIGNAL`, `traceId`, etc.) — see [glossary](../idea/glossary.md).

### Machine-local: `~/.config/paseka/<project-slug>/`

Per-colony state on this machine. Not committed.

```
~/.config/paseka/<project-slug>/
├── config.yaml          # secrets refs, NATS URL, adapter env
├── state.json           # runtime: active worktrees, last traceId, hive status
├── adapters/            # adapter-specific local overrides
│   ├── cursor.yaml      # CLI binary path, API key env
│   └── pi.yaml          # Pi CLI binary path, API key env
```

**Split rule:**

| Kind | Project `.paseka/` | Home `~/.config/paseka/<slug>/` |
| ---- | ------------------ | ------------------------------- |
| Bee roles & adapter choice | yes | — |
| Prompt templates (shareable) | yes | — |
| API keys, tokens | — | yes (or env var refs) |
| NATS connection override | — | yes |
| Active worktrees registry | pointer only | authoritative state |
| Active agent runs registry | pointer only | optional mirror in `state.json` |
| Event replay cache | — | yes |

---

## 3. Project slug

Stable identifier for the home config directory.

1. If `origin` remote exists → canonical slug from host/path (e.g. `github.com-acme-api` → `acme-api`, or full `github-com-acme-api`).
2. Else → sanitized directory name of repo root (e.g. `paseka`).
3. Collision on same machine → suffix with short hash of absolute repo path.

Stored in `.paseka/colony.yaml` as `slug` after first `paseka init` so later commands resolve the same home path.

---

## 4. `paseka init`

Run from inside a git repository (or at repo root).

```
paseka init [--adapter cursor|pi]
  │
  ├─► resolve git root (fail if not a repo)
  ├─► compute / persist project slug
  ├─► create .paseka/colony.yaml (defaults)
  ├─► create .paseka/prompts/ with starter templates (scout, builder, hivewright)
  ├─► create .paseka/bees/ with starter bees (scout, builder, hivewright) for the selected adapter
  ├─► create .paseka/.gitignore (worktrees/, runs/, *.local.yaml, cache/)
  ├─► create ~/.config/paseka/<slug>/config.yaml
  ├─► create ~/.config/paseka/<slug>/state.json (empty)
  ├─► create ~/.config/paseka/<slug>/adapters/<adapter>.yaml (cursor by default; pi when --adapter pi)
  └─► print next steps (adapter-specific auth / CLI setup, then `paseka run`)
```

`--adapter` selects which LLM adapter the starter bees use (`cursor` default; `pi` supported). Unknown adapter names fall back to `cursor`.

`paseka init` is idempotent: existing files are preserved; missing pieces are added.
