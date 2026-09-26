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
- Icons come from `lucide-svelte` and render on `currentColor`, so they inherit the semantic classes above; a hand-rolled SVG is not an accepted substitute. `StatusIcon` is the wrapper for **status** glyphs — it owns the status→glyph table and the tone — and a component that renders a *status* goes through it. A non-status affordance (a copy button, a chevron) imports the lucide component directly and applies its own sizing class.

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

- `lib/api/client.ts` is the only place that fetches. Paths are root-relative `/api/*` — the `/next` prefix belongs to frontend routes and assets, never to the API. `ApiError` carries the status **and** the server's plain-text reason (Go's `http.Error` body), because domain errors like `honey reserve not configured` are the whole point of a 400 or a 503; it falls back to `request failed: <status>` when the body is empty or unreadable.
- `consoleStatusStore` owns **one** dashboard poll (default 15s) and keeps the whole `DashboardSummary` in `store.dashboard`. The topbar derives NATS from it, and the Dashboard route reads the same object — a second poller for the same endpoint is a bug.
- A **route-scoped store** owns a different endpoint and starts on mount / stops on unmount, so an idle console keeps no poll running. `tracesStore` polls `/api/traces` (15s); `traceStore` polls `/api/traces/:id` (10s) for the detail route. The rule is one poller *per endpoint*, not one poller per app.
- A store must not assign a payload before checking that it is still the answer the operator is waiting for. Capture the id, await, and only then compare — otherwise a slow read for a trail the operator already left overwrites the current one.
- A getter is for a **derived** value only (`natsStatus` maps a raw report to a status word, `activeTraceCount` counts in-flight trails). Pass-through fields (`activeSessions`, `activeWorktrees`, `taskCounts`) are read straight off `store.dashboard` instead — a getter that only forwards one field is noise.
- `store.refresh()` forces an out-of-band poll; it is how a route updates itself after a mutation, in place of a manual Refresh button.
- `lib/clipboard.ts` owns `copyText()`. `navigator.clipboard` only exists in a secure context and the documented homelab setup is plain http on a Tailscale IP, so the selection path is a real fallback, not defensive padding — and the scratch field is always removed, including when the copy is refused.
- Formatters live in `lib/format.ts` and take plain values, not components, so they stay unit-testable. Take the narrowest structural type that covers what you read (`Pick<ArtifactView, 'announced'>`), so a formatter can be called with a literal in a test. Row timestamps use `formatTimestamp` (local, 24-hour, `YYYY-MM-DD HH:MM:SS`); `formatClock` stays for the topbar's time-of-day lines.

## Dashboard route (`/next/dashboard`)

- **No duplicate status.** The hive runtime, live bees, host, git, **and the NATS transport** live in the topbar — NATS is a `StatusIcon` beside the colony identity, so the Dashboard repeats none of it. The stat grid carries only what the topbar does not: active traces, active sessions, active worktrees, and task counts. A tile never restates a topbar panel.
- **Stat grid:** `grid-cols-1 sm:grid-cols-2 lg:grid-cols-5` — three narrow `StatTile`s (active traces, sessions, worktrees) plus a task-counts tile on `lg:col-span-2`. Five columns rather than four so all four tiles share **one** row; a tile that spans the full row always lands on a second one, which is what the legacy `stat-wide` did and what this layout deliberately drops. Active traces is the trail count the topbar deliberately does not carry.
- **Task counts** ride in the grid instead of their own card: the status is quiet text, the **count** is a `StatusBadge` taking the status tone, so `ready 1` pops green and `blocked 1` pops red while the words stay quiet. Sorted by status (`taskCountEntries`) so the tile reads the same on every poll. Empty reads `None`, not a sentence — a full clause in a tile looks like an error.
- **Recent traces** are the front-and-centre `TraceRow` list, then **Failed runs** as a `DataTable`, then **Recent insights** as `SignalCard`s — the order the legacy console used. Every trace row links to `/next/traces/<id>`; "All traces" is the only route-wide link.
- **Run cue** is a header quick-action button opening `CueRunModal` (user story #16). It is the page's only header control, so it is full-size (`btn`, not `btn-sm`) and reads as the primary action. There is no manual Refresh: the poll and the chrome stream drive the view.
- **States:** five skeletons before the first dashboard payload — one per grid column, so the grid does not reflow when the tiles arrive — then empty copy for each of the three lists and `None` in the task tile. `store.lastError` renders as an `alert-error` above the grid.

## Trace routes (`/next/traces`, `/next/traces/:id`)

- **The list is a `DataTable`, the detail is its own route.** Not a drawer: a trail carries a summary, a honey reserve, a comb, tasks, runs, a worktree, and an event tail, and a drawer cannot hold that without becoming a page anyway. A deep link to one trail must work, and `/next/traces/:id` is a child of the Traces menu entry, so `isRouteActive` keeps that entry lit on the detail route.
- **The state column speaks only when it has news.** A trail that is neither active nor failed leaves the cell empty, because `4 runs` would repeat the Runs column. The same rule governs the detail header badges: `standing` always, the state word only when there is one. A scan surface earns its keep by the rows that pop.
- **Cells never wrap.** A wrapped date or bee list doubles the row height. The one `grow` column takes the leftover width and truncates, so a long title can never push the table sideways at 390px.
- **`text` is the cell, `searchText` is the rest.** The id behind a titled row, a standing flag, and the trail summary are searchable but not shown; without `searchText` the filter would be blind to them.
- **Paging is server-side, filtering is not.** `tracesStore` keeps the pages it has pulled in and exposes `loadMore()`; a poll merges the fresh first page by id so it never drops history the operator loaded. The cursor is the last row's `lastActivityAt|traceId`, built by `traceCursor()` to match `hiveview.TraceCursorFor`.
- **Detail order is priority order:** header (back, title, badges, timeline link, summary) → Trail + Honey reserve → Trail artifacts → Tasks → Runs → Worktree → LLM usage → Recent events. Trail and Honey reserve share one `lg:grid-cols-3` row, Trail on `lg:col-span-2` — they answer "what is this trail" together, stacking two short blocks wastes a screen, and the identity grid needs the extra width because the bee list truncates without it. The trail id is the one copyable value: it is what an operator pastes into a CLI. Worktree, usage, and events are `collapsible` and start closed; they are introspection, not the reason the operator opened the trail.
- **The comb is a separate endpoint, so it fails separately.** A trail with an unreadable comb still inspects its work: `store.lastError` and `store.artifactsError` are two different alerts, and only the second one replaces the comb list.
- **Artifact bodies open in a `Modal`, never inline.** The body is fetched when the operator asks for it, and the modal owns a `requested` ref so a late answer for an artifact they already left is dropped. Markdown arrives as server-rendered HTML and is styled with Tailwind descendant utilities (`[&_h1]:…`) rather than a new dependency; anything else renders in a `<pre>`. The server's `omitted` reason is shown as-is — there is no partial body to page.
- **Top-ups patch the reserve from the response**, so a top-up never refetches the trail. The steps are disabled while one is in flight, and the outcome goes through a `Toast` rather than a blocking alert.
- **Task and run rows carry no `href` while `/next/tasks` and `/next/runs` are `PagePlaceholder`s.** A row shows state but does not promise a destination.

## Component inventory (`lib/components/`)

Use these. If a page needs a missing element, add the component here and extend this table — do not duplicate one-off markup across pages.

| Component | Use for | Notes |
| --------- | ------- | ----- |
| `StatusBadge` | Any status or state label | `badge` + one semantic class; see mapping below |
| `StatusIcon` | Status where the word is redundant (a runtime already labelled "Hive runtime") | Inline SVG on `currentColor`, `role="img"` + `aria-label` + `title` = raw status, `sr-only` text; `sm` 12px, `md` 16px, `lg` 20px |
| `DataTable` | Tabular lists: traces, bees, runs, worktrees, branches | Generic over the row type; every column supplies a `text` projection (the cell plus the filter box) and may add `searchText` for terms that are searchable but not shown; `href` renders a link, `badge` renders a `StatusBadge` (or nothing when it returns `null`), `grow` gives one column the leftover width so it truncates, `secondary` columns hide below 768px; `loading` skeletons rows, `emptyMessage` fills the body; filter resets pagination. There is no `cell` snippet — a snippet cannot be built inside `<script>`, so cell intents are declarative |
| `StatTile` | One metric in a stat grid | `label` + big `value` (`text-xl`), optional `hint` (full text for a truncating value line), optional `href` for a related route, optional `id` as a test/deep-link hook, optional `class` for a grid span, and a `children` snippet that replaces the value line (a `StatusBadge`, for instance) |
| `Section` | A titled block on a detail page | `title` + optional quiet `note` in the header, optional `actions` snippet, optional `class` for a grid span, optional `collapsible` (a `<details>` with an arrow) and `open`; `id` is a deep-link and test hook. `actions` render inside the `<summary>` too, with click propagation stopped so a control does not toggle the block |
| `MetaList` | Label/value pairs: trail identity, worktree, token usage | Stacked `dt` over `dd` in a `sm:grid-cols-2` grid; `rows` are `MetaRow`s (`label`, `value`, `mono`, `hint`, `href`, `copy`); `label` is the accessible name since a `dl` has no heading. `copy` adds an icon-only `Copy` button that swaps to a green `Check` for 1.5s, with `aria-label` and `title` naming the action — and a row that both truncates and copies needs `hint` too, or the id becomes unreadable at narrow widths |
| `DetailRow` | A compact list row: task, run, comb file | `title` + optional monospace `meta` + optional quiet `detail`, a `side` snippet for the row's badges, and an optional `href`; renders an `<li>`, so it belongs in a `<ul>` |
| `EnergyMeter` | The honey reserve of one trail | `energy` (the `HoneyReserve` fields), `pending`, `error`, and `ontopup(amount)`; renders the bar, a `low` badge, the reserve's provenance, and the fixed +1/+5/+12 steps |
| `ArtifactViewModal` | Reading one comb file | Takes `open`, `traceId`, `artifact`, `onclose`; fetches the body on open and renders markdown HTML, a `<pre>`, the server's `omitted` reason, or an error alert |
| `Modal` | Short, focused forms and destructive confirmations | Focus trap, ESC closes, restores focus to trigger; `size` is `md` for a form or `lg` for a document, the body scrolls, and the footer stays pinned outside it |
| `Drawer` | Wide or multi-step forms (launch session, task create) | Slide from right; same focus rules |
| `Toast` | Transient notifications | For action results, not persistent state; mounted once in the root layout |
| `SignalCard` | SIGNAL / INSIGHT / MUTATION / VERIFICATION presentation | Takes any `SignalSummary` — an insight highlight or an event feed row. The `severity` becomes a `StatusBadge` and the event type, payload kind, bee, and time share one source line, deduped when the type and the kind are the same word, so nothing is stated twice; `showTrace={false}` drops the trace id inside a trail, where the header already names it |
| `TraceRow` | Trail rows on the Dashboard | Title (falling back to the trace id), `runCount · taskCount · last activity` line, optional `standing` badge, `active`/`failures`/run-count state badge; the id repeats only when a title is shown, and `href` points at `/next/traces/:id` |
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
| running, interacting, connected, live, active, queued | `badge-info` |
| success, approved, merged, ready, completed, announced, open | `badge-success` |
| waiting_review, pending, reconnecting, low | `badge-warning` |
| failed, rejected, killed, blocked, cancelled, disconnected, error | `badge-error` |
| idle, stopped, stopping, unavailable, archived, planned, standing, staged, unknown | `badge-neutral` |

Task statuses are `planned`, `ready`, `running`, `waiting_review`, `completed`, `failed`, `blocked`, `cancelled`; run states are `queued`, `running`, `completed`, `failed`, `cancelled`. A comb file is `announced` or `staged`, a trail is `standing` or not, and a reserve is `low` or not — all of them have a row here so a trail surface never falls through to grey by accident.

`StatusIcon` reuses the same tone via `statusToneTextClasses` (`text-info`, `text-success`, `text-warning`, `text-error`, `text-neutral`) beside `statusToneClasses`. Glyph by status (`data-glyph` exposes it for tests): `play` for `running`/`live`/`active`/`queued`, `stop` for `stopped`, `pending` for `starting`/`stopping`, `link` for `connected`, `broken` for `disconnected`, `alert` for `failed`/`error`/`killed`, `unknown` for anything else including `idle`. Icons are lucide components: `play` = `Play`, `stop` = `Square`, `pending` = `LoaderCircle`, `link` = `Link`, `broken` = `Unlink`, `alert` = `CircleAlert`, `unknown` = `CircleDashed`. Pass `label` when the bare status is ambiguous — the topbar renders `NATS connected`, not `connected`. Pass `glyph` to force an icon regardless of status; the component then drops the status tone and the accessible name, because the surrounding control owns both.

A status missing from the table falls through to `neutral` — that is a deliberate "nothing is wrong" default, not an oversight, so a new domain status gets a row here rather than a local class in the component that shows it. Keep the tone table and the glyph list in step: a status that reads as *in flight* belongs to the `badge-info` / `play` pair.

## Hard rules (agent contract)

1. Create/edit forms open in `<Modal>` or `<Drawer>` triggered by a button — never a full-column form replacing the list view.
2. Every status is a `StatusBadge` with a semantic class from the mapping above.
3. No inline styles, no raw color literals, no custom fonts. Tailwind + DaisyUI only. The single exception is the `Hint` popover, which sets `top`/`left` from `getBoundingClientRect()` — that is geometry, not styling; it must never carry a color or a font.
4. Tabular lists are `DataTable`s (filter, paginate) — not hand-rolled `<table>` markup per page. A short list of related rows (a trail's tasks, runs, comb files) is a `<ul>` of `DetailRow`s, not a table.
5. Icon-only buttons must carry an `aria-label`; rely on DaisyUI/Tailwind focus-visible outlines.
6. Render loading (skeleton), empty, and error states — not just the happy path.
7. Stay responsive to **768px**: side menu collapses, tables never force horizontal scroll on mobile.
8. Keyboard shortcuts across routes (e.g. `g d` dashboard, `g t` traces).
9. On NATS disconnect show a reconnecting banner (`badge-warning`) and queue mutations locally; do not lose operator input. Never fire two runtime mutations at once.
10. Consume typed payloads generated from the Go event contracts; no `any`-typed event handling.
11. A collapsed `Section` must state what it holds in its summary line (a count, a caveat) so the operator can tell whether opening it is worth the click. A control rendered inside a `<summary>` stops click propagation, or it toggles the block as a side effect.

## Agent workflow

1. Read this doc + [Spec 035](../specs/035-queen-console-redesign.md) before writing UI code. Check [UI migration backlog](../plans/ui-migration-backlog.md) for a decision already deferred — a component listed in the inventory below may not exist, and a row may be deliberately inert.
2. Compose from the shell and `lib/components/`; only add a component if the inventory genuinely lacks it (and extend the table).
3. Verify with Vitest component tests and Playwright visual regression on Dashboard, Traces, Settings.
4. Theme store and route persistence must be covered by unit tests (hydration, reload persistence).

## Scope

This contract covers the **new** Svelte-based console. The current vanilla-js console under `internal/console/static/` is being replaced per Spec 035 and is out of scope for new features. Operator-facing behavior of the console lives in [docs/guide/queen-console.md](../guide/queen-console.md).