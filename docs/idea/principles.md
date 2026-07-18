# Principles

Paseka is a local, event-driven runtime for AI coding agents (bees) that work inside a git repository (colony). Agents do not share a central planner: they react to bus events and collaborate through strict message contracts.

## Choreography, not orchestration

- No central scheduler decides the next step for every agent.
- Bees subscribe to event kinds they care about and publish results back to the bus.
- Work emerges from the event stream (`SIGNAL`, `INSIGHT`, `MUTATION`, `VERIFICATION`) rather than a fixed DAG owned by a controller.

## Event contracts and identity

- Domain messages use typed contracts on NATS / JetStream.
- A **`traceId`** (flight trail) ties related signals, tasks, and mutations together.
- A **`taskId`** is a unit of nectar work inside a trace; each adapter invocation has an **`agentId`**.

## Budget and stability

- Each trace has a shared **honey reserve** (`energyToken`). Every adapter dispatch consumes energy so runaway LLM loops burn out instead of looping forever.
- Confidence / kill primitives are still design follow-ups — see [Backlog](../plans/backlog.md).

## Human in the loop

The beekeeper is a first-class participant, not a blocked approval step in an orchestrator:

- Review mutations (Queen Console / CLI).
- Interactive sessions (`paseka bee chat`).
- Top up or inspect honey for a trace.

## Colony vs machine

| Layer | Location | Holds |
| ----- | -------- | ----- |
| Colony (shareable) | `.paseka/` in the git repo | bees, prompts, colony manifest |
| Apiary (local) | `~/.config/paseka/<slug>/` | secrets, adapter credentials, runtime state |

## Where to go next

| Goal | Doc |
| ---- | --- |
| Vocabulary | [Glossary](glossary.md) |
| Historic product brief (RU) | [Brief](brief.md) |
| Config layout / `paseka init` | [Colony layout](../guide/colony-layout.md) |
| Configure a bee | [Bee config](../guide/bee-config.md) |
| CLI reference | [CLI](../guide/cli.md) |
| Shipped work & design drafts | [Changelog](../plans/changelog.md), [Specs index](../plans/specs-index.md) |
