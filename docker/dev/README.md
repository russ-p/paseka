# Paseka dev / homelab container

Minimal Ubuntu 24.04 image with:

- prebuilt `paseka` (`/usr/local/bin/paseka`), including the Queen Console Next bundle
- Go toolchain (rebuild in-container if needed)
- git
- Cursor Agent CLI (`agent`)

Default process: Queen Console on `0.0.0.0:8787`.

**Narrative and server setup:** [Homelab deployment](../../docs/guide/homelab-deployment.md).

## Quick start

From this directory:

```bash
cp .env.example .env
# edit PASEKA_REPO, PASEKA_HOME, CURSOR_API_KEY, PASEKA_NATS_URL

docker compose up --build
```

Open `http://127.0.0.1:8787` (or the host/Tailscale address you expose).

Shell instead of console:

```bash
docker compose run --rm --entrypoint bash paseka-dev
```

Rebuild the binary from the mounted repo (writes next to the sources):

```bash
docker compose run --rm --entrypoint bash paseka-dev -lc \
  'go build -o /home/dev/workspace/paseka ./cmd/paseka'
```

That binary serves the legacy console at `/` only — the redesigned console at
`/next/` is a gitignored build artifact, and the mounted repo does not carry it.
Build it on the host with `pnpm --dir web build` first, or rebuild the image so
its `web-builder` stage produces the bundle again.

Or rebuild the image so `/usr/local/bin/paseka` is refreshed.

## Environment

| Variable | Role |
| -------- | ---- |
| `CURSOR_API_KEY` | Cursor Agent authentication |
| `PASEKA_NATS_URL` | Overrides `nats.url` in home `config.yaml` |
| `PASEKA_REPO` | Host path → `/home/dev/workspace` |
| `PASEKA_HOME` | Host path → `/home/dev/.config/paseka` |
| `CURSOR_HOME` | Host path → `/home/dev/.cursor` |
| `CURSOR_CONFIG` | Host path → `/home/dev/.config/cursor` |

## Build without compose

From the **repository root**:

```bash
docker build -f docker/dev/Dockerfile -t paseka-dev:local .
```
