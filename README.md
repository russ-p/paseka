# Paseka

Paseka is a local CLI runtime for running AI agents inside a git repository. It helps solo developers initialize a colony, dispatch bees, keep runtime state, and exchange events through NATS and JetStream.

## What It Can Do

- Initialize project-local colony config in `.paseka/`.
- Run the hive runtime for a repository.
- Dispatch one-shot bee runs from the CLI.
- Start interactive human-in-the-loop bee sessions.
- Publish, replay, and inspect domain events.
- Open the local Queen Console web UI.

## How To Run

1. Clone the repository:

```
git clone https://github.com/russ-p/paseka.git
cd paseka
```

2. Download dependencies and build the binary:

```
go mod download
go build -o paseka ./cmd/paseka
```

3. Initialize a colony and start the runtime:

```
./paseka init
./paseka run
```

## Technologies

- Go
- Cobra
- NATS
- JetStream
- YAML
- PTY
