# Queen Console Design System

Short contract for agents that write Queen Console UI. Targets the Svelte 5 + Tailwind v4 + DaisyUI 5 stack from [Spec 035](../specs/035-queen-console-redesign.md). There is no bespoke design: **consistency and predictability over originality**. Read this before writing any `.svelte` file.

## Principles

1. Use ready-made DaisyUI primitives and Tailwind utilities. Never invent a new visual style when a DaisyUI class already covers it.
2. Colors, spacing, and typography come from DaisyUI themes and Tailwind — never from raw CSS literals in components.
3. A page is a **composition**: shared shell + components from `lib/components/`. New screens add composition, not new CSS.

## Theming

- Curated themes only (registered in the main stylesheet through DaisyUI's CSS-first plugin):
  `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`, `light`, `dark`.
  Do **not** add more — bundle size and predictability are the reason.
- Theme is applied via `data-theme` on `<html>`, driven by `themeStore` (Svelte store), persisted to `localStorage`.
- Dark mode is a theme switch, not `@media (prefers-color-scheme)`.
- Semantic color classes only: `base-100/200/300`, `primary`, `secondary`, `accent`, `neutral`,
  `info`, `success`, `warning`, `error`, `*-content`. No hex/RGB literals in Svelte files.
- Icons come from `lucide-svelte` and are used through `StatusIcon` only. They render on `currentColor`, so they inherit the semantic classes above; a hand-rolled SVG is not an accepted substitute.

## Layout shell

- `+layout.svelte` renders `<Header />` (top status panel), `<SideMenu />`, and `<main>`.
- Side menu is a fixed left-side panel; below **768px** it becomes a bottom sheet.
- Routes: `/next/dashboard`, `/next/traces`, `/next/timeline`, `/next/tasks`, `/next/reviews`, `/next/sessions`, `/next/bees`, `/next/worktrees`, `/next/runs`, `/next/git`, `/next/topology`, `/next/system`, `/next/settings`. The menu groups them as Work, Colony, Diagnostics, and Configuration, and each `g <key>` chord is unique. Last route + theme persist in `localStorage`.
- Header is one row: the identity block (console name, colony slug, `StatusIcon` for the NATS transport), attention badges that render only when reviews or invites are pending, then four bordered panels — Hive runtime, Live bees, Host, Git. Panel header rows are `justify-between`: label left, status right. The `/api/chrome/stream` event stream supplies runtime, agents, host, git, and attention; a dashboard poll supplies NATS. The active trail count is not in the top panel.
- The Hive runtime panel owns one icon control, not two buttons. The **icon is the action**, never the status: `runtimeAction()` returns `start` (`stopped`, `stale`), `stop` (`running`), `busy` (in-flight or `stopping`, disabled), or `choose` (anything unclear — opens a `Modal` with both actions). With a `glyph` override the icon drops the status tone and inherits the button colour, so only one colour drives it; `aria-label` spells the action out. `stopping` stays blocked because `Start` mid-shutdown would spawn a second runtime, and the result is reported with a `Toast`. Stop always opens a `Modal` confirmation.
- Every truncating line is wrapped in `Hint`: the row scrolls horizontally, so an in-flow tooltip would be clipped; the popover therefore lives in `body`. Panel header rows are `min-h-8` so a 32px icon button and a 24px badge share one label baseline, and the four panels sit in a `ml-auto` wrapper so they align to the right edge.
- The **status is the second line**: `runtimeStateNote()` is the raw status word plus a hint for the unusual values (`starting · coming up`, `stopping · shutting down`, `stale · registry entry, start respawns`, `degraded · hive reported an error`), so any status the server invents stays readable verbatim. `runtimeDetail()` (`pid · started · heartbeat`) is the third line, rendered only when it has content — the same three-line shape as the other panels.
- Below **768px** the row scrolls horizontally; panels keep their fixed widths so nothing reflows mid-scroll.
- The redesigned app is mounted at `/next/`; the legacy console remains the default at `/` until explicit cutover.

## API and store boundaries

- `lib/api/client.ts` is the only place that fetches. Paths are root-relative `/api/*` — the `/next` prefix belongs to frontend routes and assets, never to the API. `ApiError` carries the status; pages surface `error.message` and let the topbar own the stream banner.
- `consoleStatusStore` owns **one** dashboard poll (default 15s) and keeps the whole `DashboardSummary` in `store.dashboard`. The topbar derives NATS from it, and the Dashboard route reads the same object — a second poller for the same endpoint is a bug.
- A getter is for a **derived** value only (`natsStatus` maps a raw report to a status word, `activeTraceCount` counts in-flight trails). Pass-through fields (`activeSessions`, `activeWorktrees`, `taskCounts`) are read straight off `store.dashboard` instead — a getter that only forwards one field is noise.
- `store.refresh()` forces an out-of-band poll; it is how a route updates itself after a mutation, in place of a manual Refresh button.
- Formatters live in `lib/format.ts` and take plain values, not components, so they stay unit-testable. Row timestamps use `formatTimestamp` (local, 24-hour, `YYYY-MM-DD HH:MM:SS`); `formatClock` stays for the topbar's time-of-day lines.

## Dashboard route (`/next/dashboard`)

- **No duplicate status.** The hive runtime, live bees, host, git, **and the NATS transport** live in the topbar — NATS is a `StatusIcon` beside the colony identity, so the Dashboard repeats none of it. The stat grid carries only what the topbar does not: active traces, active sessions, active worktrees, and task counts. A tile never restates a topbar panel.
- **Stat grid:** `grid-cols-1 sm:grid-cols-2 lg:grid-cols-5` — three narrow `StatTile`s (active traces, sessions, worktrees) plus a task-counts tile on `lg:col-span-2`. Five columns rather than four so all four tiles share **one** row; a tile that spans the full row always lands on a second one, which is what the legacy `stat-wide` did and what this layout deliberately drops. Active traces is the trail count the topbar deliberately does not carry.
- **Task counts** ride in the grid instead of their own card: the status is quiet text, the **count** is a `StatusBadge` taking the status tone, so `ready 1` pops green and `blocked 1` pops red while the words stay quiet. Sorted by status (`taskCountEntries`) so the tile reads the same on every poll. Empty reads `None`, not a sentence — a full clause in a tile looks like an error.
- **Recent traces** are the front-and-centre `TraceRow` list, then **Failed runs** as a `DataTable`, then **Recent insights** as `SignalCard`s — the order the legacy console used. Trace rows carry no `href` until `/next/traces/:id` exists; "All traces" is the only link out.
- **Run cue** is a header quick-action button opening `CueRunModal` (user story #16). It is the page's only header control, so it is full-size (`btn`, not `btn-sm`) and reads as the primary action. There is no manual Refresh: the poll and the chrome stream drive the view.
- **States:** five skeletons before the first dashboard payload — one per grid column, so the grid does not reflow when the tiles arrive — then empty copy for each of the three lists and `None` in the task tile. `store.lastError` renders as an `alert-error` above the grid.

## Component inventory (`lib/components/`)

Use these. If a page needs a missing element, add the component here and extend this table — do not duplicate one-off markup across pages.

| Component | Use for | Notes |
| --------- | ------- | ----- |
| `StatusBadge` | Any status or state label | `badge` + one semantic class; see mapping below |
| `StatusIcon` | Status where the word is redundant (a runtime already labelled "Hive runtime") | Inline SVG on `currentColor`, `role="img"` + `aria-label` + `title` = raw status, `sr-only` text; `sm` 12px, `md` 16px, `lg` 20px |
| `DataTable` | Tabular lists: traces, bees, runs, worktrees, branches | Generic over the row type; every column supplies a `text` projection (cell text plus the filter box), `secondary` columns hide below 768px; `loading` skeletons rows, `emptyMessage` fills the body; filter resets pagination |
| `StatTile` | One metric in a stat grid | `label` + big `value` (`text-xl`), optional `hint` (full text for a truncating value line), optional `href` for a related route, optional `id` as a test/deep-link hook, optional `class` for a grid span, and a `children` snippet that replaces the value line (a `StatusBadge`, for instance) |
| `Modal` | Short, focused forms and destructive confirmations | Focus trap, ESC closes, restores focus to trigger |
| `Drawer` | Wide or multi-step forms (launch session, task create) | Slide from right; same focus rules |
| `Toast` | Transient notifications | For action results, not persistent state; mounted once in the root layout |
| `SignalCard` | SIGNAL / INSIGHT / MUTATION / VERIFICATION presentation | In feeds and detail blocks; the `severity` becomes a `StatusBadge` and the payload kind, bee, and time share the source line, so nothing is stated twice |
| `TraceRow` | Trace list rows | Title (falling back to the trace id), `runCount · taskCount · last activity` line, optional `standing` badge, `active`/`failures`/run-count state badge; the id repeats only when a title is shown, and `href` is omitted until a detail surface exists |
| `CueRunModal` | Publishing a cue (SIGNAL ingress) from any route | Fetches `/api/cues` on open, requires a cue plus non-empty text (`aria-invalid` + described hint), reports the published trace through `onran` so the caller toasts and refreshes |
| `BeeCard` | Bee / worker cards | Status, last run, adapter |
| `WorktreeCard` | Worktree rows | Branch, associated trace |
| `ThemeSelect` | Theme picker | Settings route, optional header |
| `PagePlaceholder` | Routes awaiting feature migration | Shared empty state; never a hand-rolled per-route card |
| `Hint` | Full text of a line that CSS truncates | Portaled to `body` (a scrolling ancestor would clip it), placed below the anchor and flipped above when the viewport has no room, `aria-hidden` because the visible line already holds the same text; hover and focus open it |

## Status → semantic colors

Map domain status to a DaisyUI semantic badge. Do not pick colors per context.

| Domain status | Semantic class |
| ------------- | -------------- |
| running, interacting, connected, live, active | `badge-info` |
| success, approved, merged, ready, open | `badge-success` |
| waiting_review, pending, reconnecting | `badge-warning` |
| failed, rejected, killed, disconnected, error | `badge-error` |
| idle, stopped, stopping, unavailable, archived, unknown | `badge-neutral` |

`StatusIcon` reuses the same tone via `statusToneTextClasses` (`text-info`, `text-success`, `text-warning`, `text-error`, `text-neutral`) beside `statusToneClasses`. Glyph by status (`data-glyph` exposes it for tests): `play` for `running`/`live`/`active`, `stop` for `stopped`, `pending` for `starting`/`stopping`, `link` for `connected`, `broken` for `disconnected`, `alert` for `failed`/`error`/`killed`, `unknown` for anything else including `idle`. Icons are lucide components: `play` = `Play`, `stop` = `Square`, `pending` = `LoaderCircle`, `link` = `Link`, `broken` = `Unlink`, `alert` = `CircleAlert`, `unknown` = `CircleDashed`. Pass `label` when the bare status is ambiguous — the topbar renders `NATS connected`, not `connected`. Pass `glyph` to force an icon regardless of status; the component then drops the status tone and the accessible name, because the surrounding control owns both.

A status missing from the table falls through to `neutral` — that is a deliberate "nothing is wrong" default, not an oversight, so a new domain status gets a row here rather than a local class in the component that shows it. Keep the tone table and the glyph list in step: a status that reads as *in flight* belongs to the `badge-info` / `play` pair.

## Hard rules (agent contract)

1. Create/edit forms open in `<Modal>` or `<Drawer>` triggered by a button — never a full-column form replacing the list view.
2. Every status is a `StatusBadge` with a semantic class from the mapping above.
3. No inline styles, no raw color literals, no custom fonts. Tailwind + DaisyUI only. The single exception is the `Hint` popover, which sets `top`/`left` from `getBoundingClientRect()` — that is geometry, not styling; it must never carry a color or a font.
4. Lists are `DataTable`s (filter, paginate) — not hand-rolled `<table>` markup per page.
5. Icon-only buttons must carry an `aria-label`; rely on DaisyUI/Tailwind focus-visible outlines.
6. Render loading (skeleton), empty, and error states — not just the happy path.
7. Stay responsive to **768px**: side menu collapses, tables never force horizontal scroll on mobile.
8. Keyboard shortcuts across routes (e.g. `g d` dashboard, `g t` traces).
9. On NATS disconnect show a reconnecting banner (`badge-warning`) and queue mutations locally; do not lose operator input. Never fire two runtime mutations at once.
10. Consume typed payloads generated from the Go event contracts; no `any`-typed event handling.

## Agent workflow

1. Read this doc + [Spec 035](../specs/035-queen-console-redesign.md) before writing UI code.
2. Compose from the shell and `lib/components/`; only add a component if the inventory genuinely lacks it (and extend the table).
3. Verify with Vitest component tests and Playwright visual regression on Dashboard, Traces, Settings.
4. Theme store and route persistence must be covered by unit tests (hydration, reload persistence).

## Scope

This contract covers the **new** Svelte-based console. The current vanilla-js console under `internal/console/static/` is being replaced per Spec 035 and is out of scope for new features. Operator-facing behavior of the console lives in [docs/guide/queen-console.md](../guide/queen-console.md).