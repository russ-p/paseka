# Spec 035: Queen Console Redesign

## Status

**Draft**
Initial design spec for migrating Queen Console to Svelte 5 + Tailwind v4 + DaisyUI with new layout, routing, and theming.

## Problem Statement

The current Queen Console uses an older frontend stack that limits developer experience, theming flexibility, and component composability. Operators need a modern, responsive interface with:
- Consistent theming (light/dark/custom) via DaisyUI, including Catppuccin Latte, Frappé, Macchiato, Mocha
- Clear navigation with left-side slide-out menu and top status panel
- Dedicated routes for Dashboard, Traces, Bees, Worktrees, and Settings
- Better TypeScript integration and component reuse
- Faster builds and hot-module replacement during development

## Solution

Rebuild Queen Console as a Svelte 5 application using Tailwind v4 and DaisyUI for styling. Adopt file-based routing with explicit routes and implement a persistent shell layout: top panel with cluster/NATS status indicators, left slide-out menu replacing current tabs, and a main content area per route. Extract shared UI components (cards, tables, forms, status badges) into a component library. Configure DaisyUI themes and expose a theme selector in Settings. During migration, the redesigned console is published as a preview under `/next/` while the legacy console remains the default at `/`; the root route is not redirected or replaced until feature parity and cutover approval.

## User Stories

1. As a Beekeeper, I want to switch between light, dark, and custom DaisyUI themes, so that the console matches my environment and accessibility needs.
2. As a Beekeeper, I want a top panel that always shows the NATS connection state, the hive runtime with its Start/Stop control, live bees, host, and git, so that I can assess system state at a glance without the legacy console. NATS reads as a transport icon beside the colony identity rather than a worded badge; the active trace count lives on the Dashboard.
3. As a Beekeeper, I want a left-side navigation menu with icons and labels for Dashboard, Traces, Bees, Worktrees, Settings, so that I can switch contexts without losing scroll position.
4. As a Beekeeper, I want a dedicated Dashboard route (`/next/dashboard`) showing colony overview, recent signals, and quick actions, so that I land on a useful summary when I open the redesigned console.
5. As a Beekeeper, I want a Traces route (`/next/traces`) with a filterable, paginated list of traces and a detail drawer, so that I can inspect execution history efficiently.
6. As a Beekeeper, I want a Bees route (`/next/bees`) listing all registered bees with status, last run, and adapter info, so that I can monitor bee health.
7. As a Beekeeper, I want a Worktrees route (`/next/worktrees`) showing active worktrees, their branches, and associated traces, so that I can manage isolation contexts.
8. As a Beekeeper, I want a Settings route (`/next/settings`) to configure themes, NATS endpoints, API keys, and notification preferences, so that I can customize the console without editing files.
9. As a Beekeeper, I want the console to persist my last active route and theme in localStorage, so that my preferences survive reloads.
10. As a Beekeeper, I want keyboard shortcuts (e.g., `g d` for Dashboard, `g t` for Traces) to jump between routes, so that I can navigate quickly.
11. As a Beekeeper, I want the console to gracefully degrade when NATS disconnects, showing a reconnecting banner and queuing mutations locally, so that I don't lose work during transient outages.
12. As a Beekeeper, I want the redesigned console preview to be served from the same Go binary via `paseka console` at `/next/`, so that deployment stays a single binary while I compare it with the legacy console.
13. As a developer, I want TypeScript types generated from the Go event contracts (SIGNAL, INSIGHT, MUTATION, VERIFICATION), so that frontend consumes typed payloads.
14. As a developer, I want a component storybook or visual regression setup, so that UI changes are reviewed consistently.
15. As a Beekeeper, I want the console to be responsive down to 768px width, collapsing the left menu into a bottom sheet on mobile, so that I can check status on a phone.
16. As a Beekeeper, I want forms (new task, new bee, new worktree, settings edits) to open in a modal or drawer triggered by a button, not consume a full column, so that I keep context of the list view while creating or editing.
17. As a Beekeeper, I want the legacy console to remain available at `/` throughout migration, so that unfinished redesign work cannot block current operator workflows.

## Current Section Design Audit

Each current tab audited for the redesign: what it does, which components it relies on, and a design proposal (keep as is / collapse under a button / move elsewhere).

### Dashboard

- **Functionality:** Colony-wide snapshot. Stat grid (NATS status, active sessions, active worktrees, task counts by status), recent traces, failed runs, recent insights. Quick actions: "Run cue" (opens cue modal) and "Refresh". Header panels show Hive runtime (Start/Stop), Live bees, Host, Git.
- **Components:** `stat-grid` tiles; compact lists (recent traces / failed runs / recent insights); cue modal (mnemonic picker + text form); header runtime/agents/host/git panels; toast container.
- **Design proposal:** Keep as the redesigned landing page. Collapse "Run cue" into a header quick-action button; drop the manual "Refresh" in favor of polling and the existing console event stream. Keep stat grid + recent traces front and center; move failed runs and recent insights down or push them to Traces/Timeline routes. The four header panels (Hive runtime with Start/Stop, Live bees, Host, Git) are promoted to the shared top panel rather than staying Dashboard-only, so the runtime control is reachable from every route.

### Traces

- **Functionality:** Scrollable trace list and a detail panel: meta (ID, title, status, standing), energy budget bar with top-up buttons (+1/+5/+12), LLM usage, worktree block, trail artifacts with inline preview, tasks, runs, recent events. "Open timeline" jumps to Timeline pre-filtered by the trace.
- **Components:** `session-list` rows (`TraceRow`), detail panel with collapsible `trace-section` blocks, artifact preview area, `compact-meta` definition lists, energy bar controls.
- **Design proposal:** Keep — this is the core inspection surface. Move to `/next/traces` + `/next/traces/:id` with the detail collapsing into a drawer (user story #5). Collapse energy / LLM usage / artifacts under an "Expand" accordion, since operators rarely need them on first pass.

### Timeline

- **Functionality:** Event feed of the four contracts (SIGNAL, INSIGHT, MUTATION, VERIFICATION) with filters (trace, task, bee, type, kind, severity), raw-JSON toggle, and "Load more" pagination.
- **Components:** filter bar form, feed list, raw toggle checkbox, "Load more" button.
- **Design proposal:** Keep, but collapse the filter bar under a "Filters" button to recover vertical space. Replace the raw-JSON checkbox with a per-event expand action. Consider chunked/streamed loading instead of "Load more".

### Tasks

- **Functionality:** Task creation (title, body, bee, trace ID, sector, intent, review policy, dependencies, autorun), a column-based task board, task detail (meta, description, linked runs), and approve/reject review actions (approval summary, merge commit message, PR title/body/draft/hooks).
- **Components:** full-column create form, `task-board` kanban columns, task detail panel, review action forms (approve + reject).
- **Design proposal:** This is the only tab with a full-column form — move creation into a Modal toggle on the board (user story #16) and reclaim the column for the board. Promote the board to a real kanban (columns per status). Collapse approve/PR fields under the Approve action; keep Reject minimal.

### Reviews

- **Functionality:** Review queue (tasks in `waiting_review`), proposal detail with summary, merge diff preview (stat + file list + diff2html viewer), inline review comments with drafts, approve/reject with PR creation and final merge gate; separate full-screen "Merge preview" layout.
- **Components:** queue list, proposal detail panel, merge-diff section, full-screen merge preview (file filter, file list, side-by-side/unified viewer, comments panel), review action forms.
- **Design proposal:** Keep — the merge preview + comments panel is the most complex but most important surface. Keep the full-screen diff as its own route. Rebuild on dedicated `DiffViewer` / `CommentThreads` components; gate show/hide of PR fields behind the delivery type (merge vs PR).

### Sessions

- **Functionality:** Launch interactive HITL sessions (bee, task body, optional raw prompt override, trace ID, intent), pending invites, session list, session detail with an xterm terminal (PTY via WebSocket), resume/stop, and a transcript.
- **Components:** launch form, invite list, session list, detail panel, xterm terminal with Widen toggle, transcript `<pre>`.
- **Design proposal:** Keep. Terminal interaction is space-hungry — promote "Widen" to a proper full-screen focus mode. Move the launch form into a Drawer (user story #16) and hide the resume field until a prior session is selected. Group invitations under a collapsible "Pending invites" header.

### Runs

- **Functionality:** Headless adapter runs from `paseka run` and spawned bees; detail shows metadata, summary, and a raw event list.
- **Components:** run list, run detail (meta `<dl>`, summary `<pre>`, events `<pre>`).
- **Design proposal:** Keep, compact. Consider making it a scoped filter of the Traces detail (runs for a trace) rather than a top-level tab. Render events as a readable list instead of a raw `<pre>`.

### Topology

- **Functionality:** Static colony EDA topology graph (cytoscape) derived from bee YAML and `auto_invites`; summary cards, Copy Mermaid, Refresh, Reset layout.
- **Components:** cytoscape graph container, topology summary, action buttons.
- **Design proposal:** Keep, but treat as informational — move it under Diagnostics (with System) rather than a top-level tab. Collapse Copy Mermaid / Reset layout behind an actions menu; only Refresh stays visible.

### System

- **Functionality:** OS/container view of the Queen process (PID namespace inside Docker): host identity, metrics (CPU, RSS), process table.
- **Components:** identity `<dl>`, metrics cards, `system-process-table`, Refresh.
- **Design proposal:** Keep as a Diagnostics-style page grouped with Topology under a "Diagnostics" menu section. Auto-refresh metrics periodically instead of a manual Refresh. Hide the process table under an expandable block.

### Git

- **Functionality:** Colony clone vs origin: Fetch / Push / Pull (ff-only) with hooks toggle, unpublished commits, worktrees table (Prune orphans), branches table (Delete merged leftovers).
- **Components:** origin meta `<dl>`, action buttons, worktree/branch tables, unpublished commits list.
- **Design proposal:** Keep the operational sync controls (Fetch/Push/Pull) where they are, but split the read-only worktrees list into a dedicated Worktrees route (user story #7). Group branch/worktree maintenance actions under a "Maintenance" dropdown.

## Implementation Decisions

- **Framework**: Svelte 5 with SvelteKit for file-based routing; app components use runes syntax. The global `runes` compiler flag stays off so legacy-mode dependencies such as `lucide-svelte` compile. SSR is disabled for SPA mode and the static adapter produces an embeddable fallback page.
- **Frontend isolation**: The redesign lives in a dedicated top-level frontend module with its own pnpm lockfile and build lifecycle. The legacy vanilla JavaScript console remains a separate, untouched bundle until cutover.
- **URL coexistence**: The redesigned application's base path is `/next`. `/next/` and its routes such as `/next/dashboard` are served by the new bundle; `/next` redirects to `/next/`; `/` continues to serve the legacy console. SPA fallback is confined to the `/next/` prefix so a missing preview asset cannot change legacy routing behavior.
- **Styling**: Tailwind v4 uses its CSS-first configuration and first-party Vite plugin. DaisyUI 5 is loaded from the main stylesheet. The initial theme set is limited to `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`, `light`, and `dark`.
- **Theme switching**: A `data-theme` attribute on `<html>` is driven by a Svelte store persisted to `localStorage`; DaisyUI owns the resulting CSS variables.
- **Layout shell**: The root application layout renders the top status panel, the responsive left-side menu, and the routed main content. Below 768px, the left menu becomes a bottom sheet.
- **Top panel**: Carries legacy parity as a single row. The identity block shows the console name, the colony slug, and a `StatusIcon` for the NATS transport (link glyph connected, broken glyph disconnected, dashed glyph when NATS is not configured). Four bordered panels follow — Hive runtime, Live bees, Host, and Git — each with its label, status on the right, and brief detail (runtime pid/started/heartbeat, bee counts, host CPU/memory/load, git sync state/branch/head/fetch age). Review and invite attention badges appear inline only when pending. The active trail count is not in the top panel; it belongs to the Dashboard route, fed by the same dashboard poll as NATS. The existing console status stream supplies runtime, agents, host, git, and attention; a dashboard poll supplies NATS and active trails. Queen version is deferred until the API exposes it.
- **Routing**: Application routes are Dashboard, Traces, Timeline, Tasks, Reviews, Sessions, Bees, Worktrees, Runs, Git, Topology, System, and Settings beneath the `/next` base path. The side menu groups them as Work (Dashboard, Traces, Timeline, Tasks, Reviews, Sessions), Colony (Bees, Worktrees, Runs, Git), Diagnostics (Topology, System), and Configuration (Settings); every route resolves to a `PagePlaceholder` until its section is migrated. Authentication behavior must match the existing console rather than introducing a new login boundary during the redesign.
- **API layer**: A central typed client calls the existing root-relative `/api/*` endpoints; the `/next` URL prefix applies to frontend routes and assets, not the API. TypeScript payload types are generated from Go event contracts.
- **State management**: Svelte 5 runes (`$state`, `$derived`, `$effect`) handle local state; small cross-route stores handle theme, status, and trace data.
- **Component library**: Shared DataTable, StatusBadge, StatusIcon, SignalCard, TraceRow, BeeCard, WorktreeCard, Modal, Drawer, Toast, and ThemeSelect components use DaisyUI primitives and Tailwind utilities. StatusIcon renders a `lucide-svelte` icon on `currentColor` and replaces StatusBadge where the label already names the subject, keeping the raw status as its accessible name.
- **Form pattern**: Create/edit forms use a Modal or Drawer triggered from the current list or grid; they never replace a full column. Both variants trap focus, close on Escape, and restore focus to their trigger.
- **Runtime control**: The Hive runtime panel carries a single icon control instead of Start/Stop buttons. `runtimeAction()` maps the runtime status to exactly one action: `stopped` and `stale` map to start (the supervisor clears a stale registry entry itself), `running` maps to stop, `stopping` maps to blocked, and every other value (`starting`, `degraded`, unknown, or a missing frame) maps to an explicit chooser. `stopping` must stay blocked because `Supervisor.Start` only skips spawning for `alive && running` — calling it mid-shutdown would start a second runtime. The icon is the action and inherits the button colour rather than the status tone (play, stop square, spinner, dashed circle), the accessible name spells the action out, and the panel keeps the three-line shape of its siblings: the raw status word with a hint for the unusual values on the second line, the live `heartbeat · pid · started` detail on the third. Times are always 24-hour regardless of the browser locale, and every truncating line carries a `Hint` popover with the full text on separate lines. Stop is destructive and opens a `Modal` confirmation; the ambiguous state opens a `Modal` with both Start and Stop. The returned runtime view is applied immediately instead of waiting for the next chrome frame, overlapping actions are ignored, and the result is reported through a `Toast`.
- **Build lifecycle**: The frontend exposes `dev`, `build`, `check`, and `test` pnpm scripts. Development runs at the `/next/` base path and proxies root-relative API traffic to a separately running Go console. Production build output is written into the Go embed tree and embedded independently from the legacy assets.
- **Build safety**: The generated production bundle is committed with the Go module so `go install` remains a complete single-binary deployment. A checked-in preview fallback keeps source-only Go builds compilable, while release and container builds refresh the frontend before compiling Go. The legacy bundle is never removed or redirected by the frontend build.
- **Testing**: Vitest and Testing Library cover stores and components; Playwright covers end-to-end behavior. Go route tests verify `/next` redirect behavior, preview SPA fallback, missing-asset 404s, unchanged legacy root behavior, and unchanged API routing.
- **Accessibility**: Use semantic HTML, ARIA labels for icon-only controls, visible focus states, and color-contrast-compliant DaisyUI themes.

## Testing Decisions

- Unit test each store (`themeStore`, `statusStore`) for persistence, hydration, and reactive updates.
- Component test `SideMenu` open/close, keyboard navigation, mobile breakpoint toggle.
- Component test `Header` status indicators reflect console event-stream and dashboard-poll updates, the four legacy panels render their brief detail, the runtime control offers exactly one action per state (start, stop, blocked, chooser), Stop requires modal confirmation and Escape cancels it, the chooser routes to either action, and failures surface as an alert plus an error toast.
- Unit test the topbar formatters (bytes, percent, uptime, clock, fetch age, runtime meta, git sync label), the status-to-action mapping (`runtimeAction`, `runtimeActionLabel`, `runtimeActionGlyph`, `runtimeStateNote`) across every runtime status, and the runtime mutation guards in `consoleStatusStore`.
- Component test `Modal`/`Drawer` form pattern: open on button click, trap focus, ESC closes, focus returns to trigger, list view unchanged.
- Integration test client-side navigation stays under `/next` and never captures the legacy root or root-relative API.
- E2E test happy path: preview root → dashboard → traces filter → open detail → switch theme → reload → theme persists.
- Visual regression on key pages (Dashboard, Traces list, Settings) using Playwright screenshot comparison.
- Contract test: generated TS types decode sample SIGNAL/INSIGHT/MUTATION/VERIFICATION payloads without error.
- Go routing regression tests assert that `/next` redirects to `/next/`, preview routes resolve to the new SPA fallback, missing preview assets return 404, `/` still serves the legacy console, and `/api/*` remains outside the preview mount.
- Build smoke test runs the frontend `check`, `test`, and `build` scripts before the Go build used for release artifacts.

## Out of Scope

- Real-time collaborative editing or multi-user cursors.
- Plugin/extension system for third-party panels.
- Historical analytics charts (Grafana/Prometheus integration remains separate).
- Mobile app or PWA offline support beyond localStorage persistence.
- Migration of existing console code — this is a clean rewrite.
- Replacing, redirecting, or removing the legacy console at `/` before feature parity and explicit cutover approval.

## Further Notes

- Keep the theme list at the six curated themes; add or remove themes only with an explicit design decision.
- Consider `svelte-put/clickaway` and `svelte-put/escape` for menu/drawer interactions.
- Go binary size impact: embedded SPA ~2–3 MB gzipped; acceptable for single-binary distribution.
- Delivery is phased: embeddable frontend foundation and preview route, application shell, feature-by-feature parity, then an explicit root cutover that removes or archives the legacy bundle.
- Follow-up spec may cover "Console Plugin API" once shell is stable.
