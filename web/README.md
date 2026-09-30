# Queen Console Next

Frontend foundation for the Svelte-based Queen Console redesign described in [Spec 035](../docs/specs/035-queen-console-redesign.md). The development and production app is mounted by the Go console at `/next/`; the legacy console remains at `/`.

## Layout

| Path | Holds |
| ---- | ----- |
| `src/lib/api/` | `client.ts` — the only module that fetches; `types.ts` mirrors the Go JSON views |
| `src/lib/components/` | Shared components from the design-system inventory |
| `src/lib/stores/` | Runes stores: `consoleStatusStore` (chrome stream + the one dashboard poll), `themeStore`, `toastStore` |
| `src/lib/format.ts` | Pure formatters, unit-tested |
| `src/routes/` | One directory per route; unmigrated routes render `PagePlaceholder` |
| `src/tests/` | Test-only fixtures shared across suites |

## Development

```sh
pnpm install --frozen-lockfile
pnpm dev
```

Open `http://localhost:5173/next/`. Vite proxies `/api/*` to `http://127.0.0.1:8787`; set `PASEKA_CONSOLE_API` to use another Go console address.

## Verification

```sh
pnpm check
pnpm test
pnpm build
```

The production build is written to `internal/console/next/dist/`. **It is not
committed** — it is a build artifact, and the Go binary embeds whatever is in
that directory when it compiles. A binary built from a fresh clone therefore
serves `internal/console/next/fallback.html` at `/next/` instead of the preview,
which is also why `go install` does not carry the redesign: see
[Building the binary](#building-the-binary).

## Building the binary

Build the frontend first, then Go:

```sh
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
go build -o paseka ./cmd/paseka
```

Release archives and the container image do exactly this (GoReleaser
`before.hooks`, and the `web-builder` stage in `docker/dev/Dockerfile`). A
source-only `go build` still compiles — the embed pattern is satisfied by
`next/fallback.html` alone — and the console answers `/next/` with that page
instead of the preview.

## Third-party runtime dependencies

`lucide-svelte` for icons and `cytoscape` for the colony topology graph — that is
the whole list. cytoscape is ~400 kB and only `/next/topology` draws a graph, so it
is imported dynamically inside the graph component and lands in that route's chunk
alone. Two consequences worth knowing before touching it:

- Its package `exports` map has no `types` condition, so TypeScript never finds the
  types it ships. `src/lib/cytoscape.d.ts` declares the surface the graph uses; the
  comment there explains why a `tsconfig` `paths` override is the worse fix.
- The graph reads its colours from the active DaisyUI theme at runtime rather than
  hard-coding them, so it follows the console instead of staying dark on a light
  theme. `src/lib/topology-theme.ts` owns that mapping.
