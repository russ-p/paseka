# Spec 001: Pi Adapter Integration

## Purpose

Integrate `pi` as a first-class Paseka adapter so bees can run through the Pi CLI in both AFK and interactive modes.

This spec captures the shared design only. Implementation must not start until explicitly confirmed.

## Goals

- Add adapter name `pi`.
- Support non-interactive `paseka bee run` dispatches.
- Support interactive `paseka bee chat` sessions.
- Preserve existing Cursor behavior and existing bee configs.
- Keep bus events explicit through `paseka event emit`; do not infer domain events from Pi output.

## Non-Goals

- Do not switch existing `.paseka/bees/*.yaml` files from `cursor` to `pi`.
- Do not change `paseka init` defaults or scaffold Pi config yet.
- Do not expose every Pi CLI flag through bee YAML.
- Do not parse Pi JSON into protocol bus events in the first implementation.
- Do not force Cursor-style `stream-json` semantics onto Pi.

## Current System Context

Paseka currently has two adapter surfaces:

- `adapters.Adapter` for AFK runs.
- `adapters.SessionAdapter` for PTY-backed interactive sessions.

The Cursor adapter is the only registered adapter today. It is registered in:

- `internal/runtime/dispatch.go`
- `internal/sessions/manager.go`

Bee configs select adapters through `.paseka/bees/<role>.yaml` via `adapter: cursor`, and adapter params are loaded through `internal/colony/params.go`.

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

Pi has `--approve` and `--no-approve`, but the Pi adapter will not map Paseka `trust` to these flags.

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

The first implementation uses a tolerant parser:

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

Add shared params:

- `provider`
- `thinking`

Do not add tool allowlists, extension paths, skills, themes, or broad Pi-specific session controls in this integration.

### Local Config

Add support for:

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

Do not update `paseka init` to scaffold Pi config yet.

A future `init` flag can choose or scaffold additional adapters.

## Implementation Plan

### 1. Add Pi Adapter Package

Create `internal/adapters/pi/`.

Implement AFK run behavior parallel to `internal/adapters/cursor`:

- Validate required request fields.
- Prepare run directory.
- Write prompt, metadata, and running status.
- Invoke `pi -p`.
- Capture stdout and stderr.
- Parse stdout according to `--mode`.
- Capture workspace `git diff`.
- Write protocol result and status.
- Return normalized `adapters.RunResult`.

### 2. Add Pi Session Adapter

Implement `SessionAdapter.SessionCommand`.

Interactive args should include:

- `--model`, if configured.
- `--provider`, if configured.
- `--thinking`, if configured.
- `--plan`, if configured.
- `--api-key`, if configured.
- `--session-dir <runDir>/pi-sessions`.
- `--session-id <agentId>`.
- Initial prompt as positional argument.

Interactive args should not include `--mode`.

### 3. Extend Params and Config

Update shared run params with:

- `Provider`
- `Thinking`

Update bee param parsing for:

- `provider`
- `thinking`

Allow `adapter: pi` in bee validation.

Add `PiAdapterConfig` loading under colony home config.

### 4. Register Adapter

Register `pi` in:

- `runtime.NewDispatcher`
- `sessions.NewManager`

Update dispatch and session setup so local binary/API-key config is selected by resolved adapter name:

- `cursor` uses Cursor config.
- `pi` uses Pi config.

### 5. Tests

Add focused tests for:

- Pi AFK arg building.
- Pi interactive arg building.
- Tolerant JSON summary extraction.
- Bee param parsing for `provider` and `thinking`.
- `adapter: pi` validation.
- Adapter-specific local config routing.

### 6. Docs

Update architecture docs after implementation details settle:

- Mention Pi as a supported adapter.
- Document Pi `output_format -> --mode`.
- Document `provider` and `thinking`.
- Document local `adapters/pi.yaml`.

## Verification

After Go changes:

```bash
gofmt -w .
go build -o paseka ./cmd/paseka
```

Run relevant targeted tests before the full build if the implementation touches parser, config, runtime, or sessions.
