# Paseka

[Русский](README-RU.md) · [Documentation](https://russ-p.github.io/paseka/)

Paseka is a local CLI runtime for running AI agents inside a git repository. It helps solo developers initialize a project (colony), dispatch agents (bees), keep runtime state, and exchange events over a message bus.

The product language is a hive metaphor: a **colony** (git project) of **bees** (agents) works in a **hive** (runtime), coordinating through dances on the **air** (event bus) rather than a central queen brain. See the [bee glossary](docs/idea/glossary.md).

This is a research experiment: can a swarm of modest agents coordinate through choreography alone — no central mega-brain — while keeping each bee's capability floor low enough that some can run on a local LLM on consumer hardware? Cloud models stay in the mix where the hard problems actually need them.

## What It Can Do

- Execute coding tasks — write and review code — by launching Cursor, Pi, or Claude with a focused prompt per bee role.
- Run AFK (headless) dispatches and interactive HITL sessions against the same colony.
- Operate and observe the swarm from the CLI or the local Queen Console web UI.

## How To Run

1. Clone the repository:

```
git clone https://github.com/russ-p/paseka.git
cd paseka
```

2. Download a [release binary](https://github.com/russ-p/paseka/releases) for Linux or macOS, or build from source:

```
go mod download
go build -o paseka ./cmd/paseka
```

3. Initialize a colony and start the runtime:

```
./paseka init
./paseka run
```

4. Start the local Queen Console web UI:

```
./paseka console
```

Open http://127.0.0.1:8787 in your browser. Queen Console does not enforce authentication yet, so keep it bound to localhost or another trusted interface only. Use `--addr` only when you understand the exposure risk.

## Documentation

Guides are grouped by immersion depth under [`docs/`](docs/README.md): idea → guide → reference → architecture → plans.

## Technologies

- Go
- Cobra
- NATS
- JetStream
- YAML
- PTY
