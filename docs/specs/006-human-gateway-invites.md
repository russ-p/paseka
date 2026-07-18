# Spec 006: Human Gateway Invites

## Status

**Implemented.** Platform invite lifecycle, `auto_invites`, `done_when` completion, CLI/Console surfaces, and session energy on accept are shipped.

## Purpose

Define the **platform** Human Gateway parking lot for interactive work:

- publish pending `SIGNAL/session.invite`
- Beekeeper accept/reject via CLI or Queen Console
- start an interactive session on the same `traceId`
- optionally complete invites via declarative `done_when`
- optionally auto-publish invites from colony `auto_invites` rules

This is **not** a colony choreography. Colony-owned kinds stay out of `internal/protocol` and are configured via prompts + `auto_invites`. See [005-feature-ideation-flow.md](./005-feature-ideation-flow.md) for the reference colony flow that builds on this platform.

## Goals

- Park HITL work on the bus without auto-starting PTYs from `paseka run`.
- Keep invite publish/accept/complete as reactor + invites package concerns.
- Let colonies declare when to invite (`auto_invites`) and when an accepted invite is done (`done_when`).
- Charge **1 honey** on `invite accept`; keep ad-hoc `bee chat` exempt.

## Non-Goals

- Do not AFK-dispatch interactive intents via `task.ready`.
- Do not hardcode colony kinds (`feature.*`, `spec.ready`, …) in protocol validators or reactor special-cases.
- Do not seed default `auto_invites` in the platform layer (empty means off). Reference colonies may seed rules — see [005-feature-ideation-flow.md](./005-feature-ideation-flow.md).
- Do not redesign Console beyond pending invite list / accept / reject.

## Decisions

### 1. Invite is the HITL parking lot

1. Something publishes `SIGNAL/session.invite` with `status: pending` (CLI `invite record`, Console, or `auto_invites`).
2. Beekeeper accepts via CLI or Queen Console.
3. Accept publishes `SIGNAL/beekeeper.ready` (`action: accept`) referencing `inviteId`.
4. Runtime starts a detached or attached interactive session with the invite’s `bee`, `intent`, `traceId`, and `task`.

Reject / defer updates invite to `cancelled` / `deferred`; no session.

### 2. No `dispatch: session` AFK path

Reactor must not block on a PTY inside `paseka run`. Invites project state; accept starts a session out-of-band (same process for Console, or CLI).

### 3. Platform vs colony SIGNAL boundary

| Kind | Owner | Validated by protocol? |
| ---- | ----- | ---------------------- |
| `session.invite`, `beekeeper.ready` | Platform Human Gateway | Yes, at invite boundary |
| Colony kinds used in `auto_invites` / `done_when` | Colony prompts + yaml | Envelope only |

### 4. `auto_invites` is colony config, not bee `subscribes`

Bee `subscribes` → AFK `Adapter.Run()`. `auto_invites` → pending `session.invite` only. Schema and behavior: [bee routing](../reference/bee-routing.md) §7–8.

### 5. `done_when` is a per-invite completion contract

Persisted on the invite at publish time. When a bus event matches, reactor marks that invite `completed` (required file exists) or `incomplete` (missing file). Session end without a matching completion also marks accepted invites `incomplete`.

## Event payloads

### `SIGNAL/session.invite`

```json
{
  "traceId": "trace-…",
  "agentId": "runtime",
  "type": "SIGNAL",
  "payload": {
    "kind": "session.invite",
    "inviteId": "inv-001",
    "bee": "drone",
    "intent": "grilling",
    "task": "Review: …",
    "status": "pending",
    "artifactRef": ""
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `inviteId` | yes | Stable id within the trace |
| `bee` | yes | Role to launch |
| `intent` | no | Passed to session prompt |
| `task` | yes | Initial task text |
| `status` | yes | `pending` \| `accepted` \| `cancelled` \| `completed` \| `incomplete` \| `deferred` |
| `artifactRef` | no | Repo-relative artifact handoff path |
| `doneWhen` | no | Persisted completion contract (from `auto_invites.invite.done_when`) |

### `SIGNAL/beekeeper.ready`

```json
{
  "traceId": "trace-…",
  "agentId": "beekeeper",
  "type": "SIGNAL",
  "payload": {
    "kind": "beekeeper.ready",
    "inviteId": "inv-001",
    "action": "accept"
  }
}
```

| Field | Required | Notes |
| ----- | -------- | ----- |
| `inviteId` | yes | Must match a pending invite |
| `action` | yes | `accept` \| `reject` \| `defer` |

## Surfaces

| Surface | Behavior |
| ------- | -------- |
| CLI | `paseka invite list\|record\|accept\|reject` — [CLI](../guide/cli.md) |
| Console | Pending invites; accept starts detached session |
| Telegram | Async gate — [010](./010-telegram-human-gateway.md) (MVP shipped) |
| Energy | Accept costs 1 honey from trace reserve |
| Config | `.paseka/colony.yaml` → `auto_invites` |

## Related docs

- [interactive sessions](../guide/interactive-sessions.md) — `bee chat`, session attach
- [bee routing](../reference/bee-routing.md) — `auto_invites` + `done_when` schema
- [CLI](../guide/cli.md) — invite commands
- [005-feature-ideation-flow.md](./005-feature-ideation-flow.md) — colony reference flow using this platform
- [010-telegram-human-gateway.md](./010-telegram-human-gateway.md) — Telegram Human Gateway design
- [Telegram gateway](../guide/telegram-gateway.md) — setup and run `paseka gate telegram`

## Verification

```bash
go test ./internal/protocol/... ./internal/invites/... ./internal/runtime/... ./internal/colony/...
gofmt -w .
go build -o paseka ./cmd/paseka
```
