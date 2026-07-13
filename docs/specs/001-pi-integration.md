# Spec 001: Pi Adapter Integration

## Status

**Implemented.** Pi is a first-class adapter for AFK (`paseka bee run`) and interactive (`paseka bee chat`) modes. Package: `internal/adapters/pi/`. Registered in `runtime.NewDispatcher` and `sessions.NewManager`. Machine-local config, bee params (`provider`, `thinking`, `output_format` → `--mode`), and docs are shipped. `paseka init --adapter pi` scaffolds Pi starter bees and `adapters/pi.yaml` (added after the original non-goal below).

## Purpose

Integrate `pi` as a first-class Paseka adapter so bees can run through the Pi CLI in both AFK and interactive modes.

This document remains the design record for the integration.

## Goals

- Add adapter name `pi`.
- Support non-interactive `paseka bee run` dispatches.
- Support interactive `paseka bee chat` sessions.
- Preserve existing Cursor behavior and existing bee configs.
- Keep bus events explicit through `paseka event emit`; do not infer domain events from Pi output.

## Non-Goals

- Do not switch existing committed `.paseka/bees/*.yaml` files from `cursor` to `pi` by default (opt-in via `adapter: pi` or `paseka init --adapter pi`).
- Do not change the default `paseka init` adapter away from `cursor` (Pi is opt-in via `--adapter pi`).
- Do not expose every Pi CLI flag through bee YAML.
- Do not parse Pi JSON into protocol bus events.
- Do not force Cursor-style `stream-json` semantics onto Pi.

## Current System Context

Paseka has two adapter surfaces:

- `adapters.Adapter` for AFK runs.
- `adapters.SessionAdapter` for PTY-backed interactive sessions.

Registered adapters today:

| Name | AFK | Interactive | Package |
| ---- | --- | ----------- | ------- |
| `cursor` | yes | yes | `internal/adapters/cursor` |
| `pi` | yes | yes | `internal/adapters/pi` |
| `claude` | yes | yes | `internal/adapters/claude` |
| `script` | yes | no | `internal/adapters/script` |

Registration:

- `internal/runtime/dispatch.go`
- `internal/sessions/manager.go` (LLM adapters only)

Bee configs select adapters through `.paseka/bees/<role>.yaml` via `adapter: …`, and adapter params are loaded through `internal/colony/params.go`. Canonical docs: [003-architecture.md](../003-architecture.md) §5.1, [006-interactive-sessions.md](../006-interactive-sessions.md) §8, [010-bee-config.md](../010-bee-config.md).

## Pi CLI Facts

The local Pi CLI is available as `pi`.

Relevant flags:

- `-p`, `--print`: non-interactive mode.
- `--mode <mode>`: output mode, with `text`, `json`, or `rpc`.
- `--model <pattern>`: model selection.
- `--provider <name>`: provider selection.
- `--thinking <level>`: thinking level.
- `--api-key <key>`: explicit API key.
- `--plan`: extension flag for plan mode.
- `--session-dir <dir>`: session storage directory.
- `--session-id <id>`: exact project session id.

Pi has `--approve` and `--no-approve`, but the Pi adapter does not map Paseka `trust` to these flags.

## Decisions

### Adapter Scope

Implement both adapter surfaces:

- AFK: `pi -p`.
- Interactive: `pi` under Paseka-owned PTY.

### AFK Output

AFK runs default to:

```bash
pi -p --mode json "$PROMPT"
```

`params.output_format` maps to Pi `--mode` when it is one of:

- `text`
- `json`
- `rpc`

If `params.output_format` is empty, the adapter uses `json`.

AFK uses a tolerant parser:

- Extract summary/output from common JSON fields when present.
- Preserve raw stdout JSON as an artifact.
- Do not treat Pi JSON as bus events.
- Continue to rely on agents calling `paseka event emit` for domain events.

### Interactive Output

Interactive sessions do not pass `--mode`.

This keeps Pi in its normal interactive UI for `paseka bee chat`.

### Session Storage

Interactive Pi sessions use run-local storage:

```bash
pi --session-dir "<runDir>/pi-sessions" --session-id "<agentId>" "$PROMPT"
```

This keeps Pi session artifacts tied to the Paseka run directory and the current `agentId`.

### Trust and Force

The Pi adapter does not map existing Paseka `trust` to Pi `--approve` or `--no-approve`.

The Pi adapter ignores `force`, because Pi has no equivalent flag.

### Supported Bee Params

Reuse existing params:

- `model`
- `output_format`
- `plan`
- `binary`

Shared params:

- `provider`
- `thinking`

Do not add tool allowlists, extension paths, skills, themes, or broad Pi-specific session controls in this integration.

### Local Config

Support:

```yaml
binary: pi
api_key_env: GEMINI_API_KEY
```

Location:

```text
~/.config/paseka/<slug>/adapters/pi.yaml
```

If the file is missing, default to:

```yaml
binary: pi
```

If `api_key_env` is configured and the environment variable is set, pass the value to Pi with `--api-key`.

### Init Behavior

`paseka init --adapter pi` scaffolds starter bees with `adapter: pi` and creates `adapters/pi.yaml`. Default init remains `cursor`.

## Implementation (shipped)

### 1. Pi Adapter Package

`internal/adapters/pi/` — AFK run parallel to Cursor:

- Validate required request fields.
- Prepare run directory.
- Write prompt, metadata, and running status (including PID).
- Invoke `pi -p`.
- Capture stdout and stderr.
- Parse stdout according to `--mode`.
- Capture workspace `git diff`.
- Write protocol result and status.
- Return normalized `adapters.RunResult`.

### 2. Pi Session Adapter

`SessionAdapter.SessionCommand` with:

- `--model`, `--provider`, `--thinking`, `--plan`, `--api-key` when configured.
- `--session-dir <runDir>/pi-sessions`.
- `--session-id <agentId>`.
- Initial prompt as positional argument.
- No `--mode`.

### 3. Params and Config

- Shared run params: `Provider`, `Thinking`.
- Bee param parsing for `provider` / `thinking`.
- `adapter: pi` allowed in bee validation.
- `PiAdapterConfig` under colony home config.

### 4. Registration

- `runtime.NewDispatcher` and `sessions.NewManager` register `pi`.
- Local binary/API-key config selected by resolved adapter name (`cursor` vs `pi` vs `claude`).

### 5. Tests

Covered in `internal/adapters/pi/`, colony params/validation, and runtime dispatch tests.

### 6. Docs

Documented in architecture, interactive sessions, bee config, and CLI (`paseka init --adapter pi`).

## Verification

```bash
gofmt -w .
go build -o paseka ./cmd/paseka
go test ./internal/adapters/pi/ ./internal/colony/ ./internal/runtime/ -count=1
```
