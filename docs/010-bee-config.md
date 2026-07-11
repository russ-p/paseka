# Bee role config (`.paseka/bees/<role>.yaml`)

A **bee** is a named role bound to an adapter, prompt template, and optional routing / completion rules. Each file under `.paseka/bees/` defines one role.

Implementation: [`internal/colony/bee.go`](../internal/colony/bee.go) (`Bee` struct, `LoadBee`), plus [`command.go`](../internal/colony/command.go), [`params.go`](../internal/colony/params.go), [`routing.go`](../internal/colony/routing.go), [`run_summary.go`](../internal/colony/run_summary.go), [`completion.go`](../internal/colony/completion.go), [`bee_validate.go`](../internal/colony/bee_validate.go).

Related: [008-bee-routing.md](008-bee-routing.md) (`subscribes` / `publishes`), [004-prompt-templates.md](004-prompt-templates.md), [003-architecture.md](003-architecture.md) (adapters, colony layout).

---

## 1. Files and loading

```
.paseka/bees/
├── scout.yaml
├── builder.yaml
├── guard.yaml
└── builder.local.yaml   # optional, gitignored overlay
```

| Path | Purpose |
| ---- | ------- |
| `.paseka/bees/<role>.yaml` | Canonical role definition (committed) |
| `.paseka/bees/<role>.local.yaml` | Machine-local overlay; `prompt_template` and `system_template` applied at resolve time |

`paseka` loads bees via `colony.LoadBee(colonyRoot, role)` / `LoadAllBees`:

1. Role must be non-empty and must not contain `/` or `..`.
2. Base file is `.paseka/bees/<role>.yaml` (filename stem = role when `role:` is omitted).
3. Event rules, `run_summary`, and adapter requirements are validated at load time.
4. If `<role>.local.yaml` exists, its `prompt_template` and `system_template` override the base at resolve time (see [004-prompt-templates.md](004-prompt-templates.md)).

`*.local.yaml` files are listed in `.paseka/.gitignore` and are skipped by `LoadAllBees`.

---

## 2. Schema

Go type (`internal/colony/bee.go`):

```go
type Bee struct {
    Role               string
    Adapter            string
    PromptTemplate     string
    SystemTemplate     string
    Sector             string
    Worktree           bool
    Intents            []string
    DefaultIntent      string
    Command            Command
    PostExec           Command
    Params             map[string]any
    Subscribes         []SubscriptionRule
    Publishes          []PublicationRule
    CompletionContract CompletionContract
    RunSummary         RunSummaryPolicy
}
```

### Field reference

| YAML field | Required | Meaning |
| ---------- | -------- | ------- |
| `role` | recommended | Role name. If empty, defaults to the filename stem (`builder.yaml` → `builder`). |
| `adapter` | no | `cursor` (default), `pi`, `claude`, or `script`. Unknown names fail load. |
| `prompt_template` | usually | Path relative to `.paseka/prompts/`. User/task turn. Optional for `adapter: script` (no colony default applied when omitted). |
| `system_template` | no | Path relative to `.paseka/prompts/`. Role / standing instructions injected by the adapter (see [004-prompt-templates.md](004-prompt-templates.md)). |
| `sector` | no | Default sector name from `colony.yaml` `sectors`. Task `sector` wins when set. |
| `worktree` | no | When `true`, adapter cwd is under `.paseka/worktrees/<traceId>/` (plus sector path if any). |
| `intents` | no | Explicit intent vocabulary for this bee. When omitted, runtime discovers intents from `_partials/<role>-intent-*.md` prompt partials. |
| `default_intent` | no | Default intent when the caller omits `--intent` or passes an unknown value. When omitted, `general` is used if present in the vocabulary; otherwise the first discovered intent. |
| `params` | no | Adapter flag map (`model`, `trust`, …). Ignored when `command` is set (runtime warns if both are present). |
| `command` | script: **yes** | Full agent argv (string or YAML list). Replaces `params`-based flag mapping. |
| `post_exec` | no | Hook after AFK `bee run` and interactive `bee chat`. Failures are logged; they do not fail the bee run. |
| `subscribes` | no | Event → dispatch rules. Empty = backward-compatible allow any `task.ready`. See [008-bee-routing.md](008-bee-routing.md). |
| `publishes` | no | Advisory expected outputs; undeclared domain publishes warn only (MVP). |
| `completion_contract` | no | Hard post-run event requirements; violation fails the run. |
| `run_summary` | no | `auto` (default) \| `required` \| `disabled` — controls `INSIGHT/run.summary` synthesis/enforcement. |

---

## 3. Example

```yaml
# .paseka/bees/builder.yaml
role: builder
adapter: cursor
sector: frontend
params:
  model: composer-2.5
  output_format: stream-json
  trust: true
  force: true
# Optional: override adapter flag mapping (docker-compose style).
# command: agent -p --yolo --workspace $WORKSPACE $PROMPT
prompt_template: builder.md   # relative to .paseka/prompts/
worktree: true                  # run inside .paseka/worktrees/<traceId>/
subscribes:
  - type: SIGNAL
    kind: task.ready
    dispatch: task
publishes:
  - type: MUTATION
    kind: code.proposal
```

Guard with a completion contract:

```yaml
# .paseka/bees/guard.yaml
role: guard
adapter: cursor
prompt_template: guard.md
params:
  model: composer-2.5
  output_format: stream-json
  trust: true
  force: true
worktree: true
subscribes:
  - type: MUTATION
    kind: code.proposal
    dispatch: direct
publishes:
  - type: VERIFICATION
    kind: verification.success
  - type: VERIFICATION
    kind: verification.failed
completion_contract:
  required:
    - type: VERIFICATION
      kind_one_of:
        - verification.success
        - verification.failed
      count: 1
```

---

## 4. Adapters

`ResolveAdapter()` defaults empty `adapter` to `cursor`. Allowed values: `cursor`, `pi`, `claude`, `script`.

| Adapter | Notes |
| ------- | ----- |
| `cursor` | Cursor Agent CLI (`agent`). Params map to CLI flags unless `command` is set. Custom `command` with `system_template` should pass `--plugin-dir $CURSOR_PLUGIN` for system injection. |
| `pi` | Pi CLI (`pi`). Params: `model`, `provider`, `thinking`, `output_format`, `plan`, `binary`. |
| `claude` | Claude Code CLI; same params plumbing as other LLM adapters. |
| `script` | **Requires** `command`. AFK-only (`bee run`); `bee chat` is LLM-only. `params` ignored. `prompt_template` optional. |

Adapter drivers and flag mapping live in [003-architecture.md](003-architecture.md) §5. Machine-local credentials stay in `~/.config/paseka/<slug>/adapters/*.yaml`.

Script bee example:

```yaml
# .paseka/bees/oracle-guard.yaml
role: oracle-guard
adapter: script
command: ./scripts/oracle-guard.sh
run_summary: disabled
subscribes:
  - type: MUTATION
    kind: code.proposal
    dispatch: direct
publishes:
  - type: VERIFICATION
    kind: verification.success
  - type: VERIFICATION
    kind: verification.failed
```

Script process env (in addition to `command` variable substitution): `PASEKA_TRACE_ID`, `PASEKA_AGENT_ID`, `PASEKA_TASK_ID`, `PASEKA_WORKSPACE`, `PASEKA_COLONY_ROOT`, `PASEKA_RUN_DIR`, `PASEKA_BEE`, `PASEKA_EVENT_LOG`, `PASEKA_RESULT_FILE`, `PASEKA_PROMPT_FILE`. Domain events still go through `paseka event emit --stdin`.

---

## 5. `params`

Mapped by `RunParamsFromBee` (`internal/colony/params.go`). Defaults: `trust: true`, `force: true`.

| Key | Type | Used by |
| --- | ---- | ------- |
| `model` | string | cursor, pi, claude |
| `output_format` | string | cursor (`stream-json`, …); pi maps to `--mode` |
| `trust` | bool | cursor |
| `force` | bool | cursor |
| `plan` | bool | cursor (`--plan`); pi (`--plan`) |
| `binary` | string | override CLI binary name |
| `provider` | string | pi |
| `thinking` | string | pi |

When `command` is set, these params are **not** turned into CLI flags; runtime logs a warning if both `command` and `params` are present. `adapter` still selects result parsing, session PTY, and home-config credential injection.

---

## 6. `command` and `post_exec`

Both accept a shell-like string or a YAML list of strings (`colony.Command`). String form is split into argv without invoking a shell (quotes supported; unclosed quotes error).

```yaml
command: agent -p --trust --workspace $WORKSPACE $PROMPT
# or
command: ["agent", "-p", "--model", "composer-2.5", "$PROMPT"]
```

```yaml
post_exec: notify.sh --bee builder --status ok --summary "$RESULT"
# or
post_exec: ["curl", "-fsS", "-d", "@$META", "https://hooks.example.com/paseka"]
```

### Variable substitution

Supports `$NAME` and `${NAME}`:

| Variable | When set | Value |
| -------- | -------- | ----- |
| `$PROMPT` / `${PROMPT}` | dispatch + post_exec | rendered user/task prompt |
| `$SYSTEM_PROMPT` / `${SYSTEM_PROMPT}` | dispatch + post_exec | rendered system prompt |
| `$SYSTEM_FILE` / `${SYSTEM_FILE}` | dispatch + post_exec | path to `system.txt` |
| `$CURSOR_PLUGIN` / `${CURSOR_PLUGIN}` | dispatch + chat | ephemeral Cursor plugin dir (`.paseka/runs/<traceId>/<agentId>/cursor-plugin`); pass as `--plugin-dir` in custom Cursor `command` |
| `$WORKSPACE` / `${WORKSPACE}` | dispatch + post_exec | agent working directory |
| `$TRACE_ID` / `${TRACE_ID}` | dispatch + post_exec | current flight trail |
| `$AGENT_ID` / `${AGENT_ID}` | dispatch + post_exec | this invocation id |
| `$TASK_ID` / `${TASK_ID}` | dispatch + post_exec | task id when dispatched from ledger |
| `$COLONY_ROOT` / `${COLONY_ROOT}` | dispatch + post_exec | git repo root |
| `$RUN_DIR` / `${RUN_DIR}` | dispatch + post_exec | `.paseka/runs/<traceId>/<agentId>/` |
| `$RESULT_FILE` / `${RESULT_FILE}` | dispatch + post_exec | path to `result.txt` |
| `$RESULT` / `${RESULT}` | post_exec only | human-readable run summary text |
| `$META` / `${META}` | post_exec only | path to `meta.json` |

---

## 7. Sector and worktree

- **`sector`** — default named path from `colony.yaml` `sectors`. Effective sector = task sector if set, else bee default (`EffectiveSector`). Workspace becomes `colonyRoot/<sector.path>` or worktree + sector path when `worktree: true`.
- **`worktree: true`** — mutations under `.paseka/worktrees/<traceId>/`; audit I/O stays in `.paseka/runs/`.

Colony sector definitions remain in [003-architecture.md](003-architecture.md).

---

## 8. `run_summary`

Controls runtime handling of `INSIGHT/run.summary`:

| Value | Behavior |
| ----- | -------- |
| `auto` (default / empty) | Runtime may synthesize a summary when missing and policy allows |
| `required` | Run fails if no summary event is present after the adapter exits |
| `disabled` | No synthesis; useful for script / oracle bees |

Invalid values fail bee load. See also [008-bee-routing.md](008-bee-routing.md) §5 and [009-insight-kinds.md](009-insight-kinds.md).

---

## 9. `completion_contract`

Hard requirements checked against `events.ndjson` after the adapter exits. Violation → run **failed** even if the process exit code was zero.

```yaml
completion_contract:
  required:
    - type: VERIFICATION
      kind_one_of:
        - verification.success
        - verification.failed
      count: 1   # default 1; must match exactly that count among allowed kinds
```

| Field | Meaning |
| ----- | ------- |
| `type` | Domain event type (`SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION`) |
| `kind_one_of` | Allowed `payload.kind` values (required, non-empty) |
| `count` | Exact match count among those kinds (default `1`) |

Narrative INSIGHTs do not satisfy contracts unless listed. Full routing semantics: [008-bee-routing.md](008-bee-routing.md) §6.

---

## 10. Routing fields (`subscribes` / `publishes`)

Documented in [008-bee-routing.md](008-bee-routing.md). Summary:

- `subscribes[].dispatch`: `task` (task-ledger) or `direct` (reactor runs the bee on the event).
- Empty `subscribes` → any `task.ready` dispatch allowed.
- `publishes` is advisory in MVP.

---

## 11. Prompt template resolution

Precedence (highest wins), from [004-prompt-templates.md](004-prompt-templates.md):

1. Inline `prompt:` / CLI `--prompt`
2. `bees/<role>.local.yaml` → `prompt_template`
3. `bees/<role>.yaml` → `prompt_template`
4. `colony.yaml` → `defaults.prompt_template`

Do not store prompts in `~/.config/paseka/`.
