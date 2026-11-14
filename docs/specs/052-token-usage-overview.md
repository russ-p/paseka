# Spec 052: Token usage overview

## Status

**(Implemented)**
Colony-wide, time-bucketed LLM token usage over retained trail records: one read model, a Console page with daily charts, and a `paseka inspect usage` range mode. The four product choices left open in the draft — recorded cost placement, default range, the Bee axis, and drill-down behaviour — were settled by the Beekeeper and are recorded under Further Notes.

Shipped in trail `trace-01a1da38673703e4` as one read model with three readers and no stored aggregate. `runs.ReadUsage` derives the report from the existing run projection; `GET /api/usage` serves it read-only and refuses an invalid range with a plain-text 400 rather than an empty report; a 30-second scan cache keeps repeat reads cheap while `?refresh=1` bypasses it for the page's refresh control; `paseka inspect usage` without `--trace` prints the same report as text or `--json`, with `--group-by` choosing the ranked axis and repeatable `--adapter`/`--model`/`--bee`/`--purpose` filters that never move the range; and the Console Usage page shows the daily chart, the axis breakdown, the top trails, filters, and drill-down from a chart column or breakdown row into the URL state, so a narrowed view is a link. Coverage and honesty are stated rather than implied: absent usage reads as missing instead of zero, an in-flight run marks its day incomplete, a range older than the retained record warns, and a pruned trail is absent instead of reported as zero spend.

## Problem Statement

A Beekeeper who pays for the colony's LLM traffic can see what one trail cost, but cannot see the colony's consumption over time. The three existing reads are all single-trail or single-run:

- `paseka inspect usage --trace <id>` sums tokens for one trail and prints input, output, and cache tokens — no model, no adapter, no time axis.
- `paseka inspect metrics --trace <id>` gives a rich per-trail projection, but only for that trail.
- The Console's Runs route lists the most recent runs (capped at 50), each with its own usage number; the Traces route shows a per-trail aggregate.

So the question "what did we spend last week, and on which models and adapters" is answered today by hand-written `jq` over `.paseka/runs/` — the collector that [043b](043b-trail-metrics-and-work-completion.md) replaced for a single trail and left to the operator across trails.

Four consequences follow:

1. **No time series.** Tokens are recorded per execution and never bucketed by day, so a spike is indistinguishable from steady growth.
2. **The model mix is invisible.** Model aliases, pinned models, and adapter versions change between weeks, and nothing compares them. A standing trail that switched from a cheap model to an expensive one looks like ordinary activity.
3. **No adapter comparison.** Adapters report usage differently — one reports cache reads, another reports cost, Cursor HITL reports nothing — and a colony-wide number that hides this reads as a comparison it is not.
4. **No honesty surface.** An execution without reported usage is not a free execution, and a pruned trail's tokens are not zero. A total that silently omits both is wrong in the one direction an operator will act on.

## Solution

One colony-wide usage read model, built in Go from retained run records, served two ways:

1. **Console page "Usage"** — a range picker (7 / 30 / 90 days or custom), one daily chart, a period breakdown by adapter, model, Bee, or execution purpose, a top-trails table, and an explicit coverage line. Charts are drawn with LayerChart from DaisyUI theme tokens behind one in-repo wrapper component.
2. **`paseka inspect usage --since … [--until …] [--json]`** — the same report from the same reader, so the page is never the only way to get the numbers, and the aggregation is testable without a browser.

The report is an observation over **retained** records: it says which days it can see, how many executions reported usage, and when the retained record begins. It never invents a price, never fills a missing measurement with zero, and never turns a token count into money.

## User Stories

1. As a Beekeeper, I want to see the colony's token consumption per day for a chosen period, so that I can tell a spike from steady growth.
2. As a Beekeeper, I want to choose 7, 30, or 90 days or an explicit date range, so that I can answer both "this week" and "since the model switch".
3. As a Beekeeper, I want each day split into input, output, reasoning, cache-read, and cache-write tokens, so that I can see whether a total grew because of work or because of context replay.
4. As a Beekeeper, I want to switch the daily chart to a breakdown by model, so that a change in model mix is visible on the time axis.
5. As a Beekeeper, I want to switch the daily chart to a breakdown by adapter, so that I can see which execution path the colony actually spends on.
6. As a Beekeeper, I want the adapter breakdown to distinguish an adapter from its implementation version, so that an `opencode` proxy run on v1 and one on v2 are not added together as one thing.
7. As a Beekeeper, I want a breakdown by Bee, so that one expensive role cannot hide inside a colony total.
8. As a Beekeeper, I want a breakdown by execution purpose (work, closing, analysis), so that study and wrap-up overhead do not read as goal work — the same split the trail metrics already make.
9. As a Beekeeper, I want to filter by adapter, model, Bee, and purpose, so that I can isolate the subset I am paying for.
10. As a Beekeeper, I want a period table ranked by the selected dimension with each key's share, so that I can name the top two or three consumers without reading the chart.
11. As a Beekeeper, I want a top-trails table for the period, so that I can open the trail whose runs produced the most tokens.
12. As a Beekeeper, I want the period total and its component split as the headline number, so that I do not have to add up the chart.
13. As a Beekeeper, I want a coverage line telling me how many executions reported usage and when the retained record begins, so that I know whether the total is the colony's or only the visible part of it.
14. As a Beekeeper, I want a day that has runs but no reported usage to look different from a day with no runs at all, so that a quiet week and an unmeasured week are not the same picture.
15. As a Beekeeper, I want a run that is still in flight to be visible as unfinished rather than as free, so that today's number is not read as a final figure.
16. As a Beekeeper, I want token counts labelled as volume rather than as money, so that I do not compare a cache-heavy model with an expensive one by token count alone.
17. As a Beekeeper, I want recorded cost in dollars shown where an adapter reports it, with its own coverage, so that I can use the provider's own number when it exists.
18. As a Beekeeper, I want the page to refuse to compute price from a token table, so that a stale or invented rate never appears as a colony cost.
19. As a Beekeeper, I want the range and filters in the URL, so that a view of an expensive week is a link I can send or bookmark.
20. As a Beekeeper, I want the page to load without waiting for a live poll, so that a history page does not keep a filesystem walk running in the background.
21. As a Beekeeper, I want an explicit refresh action, so that a newly finished trail can appear without reloading the browser.
22. As a Beekeeper, I want the chart's data also available as a table, so that I can read exact numbers and copy them.
23. As a Beekeeper, I want an accessible page, so that the chart is not the only way to read the period.
24. As a Beekeeper, I want the same numbers from the CLI as JSON, so that I can script a monthly report or check the page against its source.
25. As an operator, I want a clear refusal when the range is invalid, so that a typo cannot produce a silently empty report.
26. As an operator, I want days zero-filled across the range, so that two ranges with different activity still have a comparable x-axis.
27. As an operator, I want runs with an unreadable or absent start time reported as excluded rather than dropped, so that the total is honest about what it could not place.
28. As an operator, I want the report to say it covers retained trails only, so that a prune does not silently rewrite history.
29. As an operator, I want a trail whose worktree was pruned but whose run records remain to still count, so that cleanup of checkouts does not hide spend.
30. As an operator, I want one execution that reports usage through two records to be counted once, so that an interactive session does not double the bill.
31. As an operator, I want an unknown model or adapter to appear as an explicit unknown group, so that legacy runs are counted and named rather than vanishing from the breakdown.
32. As a maintainer, I want the aggregation to reuse the existing run projection, so that the page and `inspect usage` cannot disagree about what a run reported.
33. As a maintainer, I want the day boundary defined by an explicit timezone, so that two operators in different zones do not get different totals for the same range.
34. As a maintainer, I want a bounded range, so that one request cannot turn into an unbounded walk of every run directory ever written.
35. As a Beekeeper, I want the page to remain readable on a narrow window, so that checking yesterday's spend from a laptop works.
36. As a Beekeeper, I want clicking a day or a breakdown row to narrow the page in place, so that I can follow a suspicion without losing the range and the other filters.
37. As a Beekeeper, I want the cost figure to stay out of the headline, so that a partial, adapter-dependent dollar sum is never the first number I read.

## Implementation Decisions

### 1. One read model over retained run records

1. A colony-wide usage projection is added to the run-record reader beside the existing run and trail projections. It reads `.paseka/runs` the way the unbounded run scan already does, and produces one record per retained execution with: trail identity, run identity, Bee, adapter, implementation, resolved model and model alias, execution purpose, start time, recorded usage, and the measurement state of that usage.
2. The reader is **not** a second interpreter of run files. Adapter, model, purpose, start time, and usage precedence come from the existing run projection, which already resolves `status.json` over `request.json`, prefers `result.json` usage, falls back to `session.json` usage for interactive sessions, and fills a missing model from the session. A page that disagreed with `inspect usage` about the same run would be a bug in the projection, not in the page.
3. Usage fields are the recorded ones: input, output, reasoning, cache read, cache write, recorded cost, and the usage source. Reasoning tokens stay a separate component and are **never** added to output, because providers differ on whether reasoning is already inside it; the same reason cache kinds are not folded into input.
4. `total` means "the sum of the components this record reported", and is only printed next to its components. No single total is presented as billable tokens.
5. One retained execution is one record. A directory that carries both AFK result usage and interactive session usage contributes one measurement, using the projection's existing precedence. A resumed session is a new execution in its own run directory and therefore counts separately — it is new provider traffic, not a duplicate.
6. `.paseka/runs/<traceId>/tasks/` and other non-execution directories are not executions and never enter the report.
7. Experimental forks and no-delivery trails are ordinary spend and are included. Fork lineage and configuration attribution remain the trail metrics' business, not this report's.

### 2. Time buckets

1. Buckets are local calendar days in an explicit timezone supplied by the caller. The API accepts an IANA zone name; the Console sends the browser's zone; the CLI defaults to the host's local zone and accepts an override. The report echoes the zone it used.
2. A record is placed by its **start** time, not its finish time. Usage is reported once for the whole execution, so splitting it across a midnight boundary would invent a distribution the record does not contain. A run that crosses midnight lands entirely on its start day, and the page says so.
3. `since` is inclusive and `until` exclusive, both interpreted as local midnights in the chosen zone. A missing `since` defaults to 30 days ending now; a missing `until` defaults to now.
4. The range is capped (366 days). A longer request is a client error, not a silently truncated report.
5. Days with no executions are zero-filled so that the axis is continuous. A day with no executions is a genuine zero; a day whose executions reported no usage is *unmeasured* and is a different state in both the API and the chart. The two must never render identically.
6. A record with no usable start time cannot be bucketed. It is counted in a separate excluded total with a warning, never dropped and never assigned a guessed date.

### 3. Measurement states and coverage

1. Every record carries a measurement state: `reported`, `missing` (the run finished without a usage record), or `unknown` (the run is in flight, or its outcome record is not readable yet). Coverage counts each state; a total over `reported` records is a total over observed traffic only.
2. The report carries the retained-record start (`earliestRetainedAt`). When the requested range begins before it, the report warns that earlier days are not measured; it does not claim they were quiet.
3. Records are read from disk, so a pruned trail's tokens are gone. The report states that it covers retained trails, and nothing reconstructs deleted spend. A trail whose worktree was pruned but whose run data remains still counts.
4. A day containing at least one in-flight execution is marked incomplete. The page names today as such rather than waiting for a poll to make it true.
5. Report totals never invent a number for an unreadable source. Unreadable directories produce warnings and a reduced coverage count, not zeros.

### 4. Dimensions and filters

1. Four axes are aggregated simultaneously: adapter, model, Bee, and purpose. Each axis sums to the same day total, so the UI can switch axes without a second request and without a cross-tab.
2. The adapter key is the recorded adapter name; its implementation version is shown beside it when recorded and different. This keeps an `opencode` proxy run and its `opencodev2` implementation legible instead of merging them.
3. The model key is the resolved model identifier. The recorded alias is shown as a secondary label; grouping by alias instead is rejected because an alias is colony configuration that changes meaning, while the resolved id is what was actually called.
4. Missing model, adapter, or Bee values group under explicit `unknown` keys, so legacy records are counted and visible rather than excluded.
5. Purpose uses the recorded `work`, `closing`, and `analysis` vocabulary. A legacy record without a purpose is its own `unknown` group, not silently `work` — the same rule [043b](043b-trail-metrics-and-work-completion.md) applies.
6. Filters (adapter, model, Bee, purpose) restrict records before aggregation and may be repeated. Filters do not change the range or the day boundaries.
7. A cross-tabulation (model × adapter, for example) is deliberately not offered: the recorded volume is additive along one axis at a time, and a cross-tab would encourage splitting a total that the records cannot justify.

### 5. Report contract

1. `GET /api/usage` returns one JSON report: the echoed range and timezone, `observedAt`, `earliestRetainedAt`, period totals, one entry per day in the range, the excluded-undated count, and warnings.
2. Each day carries: local date, execution count, count with reported usage, distinct trail count, the token component totals, recorded cost and the number of executions that reported it, and one array per axis (key, executions, tokens, cost) whose sums equal the day's totals.
3. Period totals and the per-axis period ranking are derived from the day entries rather than recomputed, so a period figure cannot disagree with the days under it.
4. The endpoint is read-only and changes no files, publishes no events, and needs no bus. Invalid range, unknown timezone, unparsable date, and over-long range are `400`s with a plain-text reason in the console's existing style; a missing `.paseka/runs` is an empty report, not an error.
5. Recorded cost is summed only where an execution reported it, and is always accompanied by its own reporting count. It appears in breakdown rows and the daily table and never as the headline figure. No price table, rate card, currency conversion, or estimate exists anywhere in the reader, the API, or the page.
6. The reader may cache its parsed scan in process for a short interval, because the page is history rather than live telemetry and a filesystem walk per keystroke is not acceptable. A cached report must carry the `observedAt` of the scan it came from, and an explicit refresh must be able to bypass the cache. No aggregate is persisted to disk.

### 6. Console page

1. A new route `usage` is added to the Console navigation in the Colony group, with its own glyph and an unused `g <key>` chord, following the existing route table rather than a one-off link.
2. Page composition: a range control (7 / 30 / 90 days and a custom start/end), filter chips for adapter, model, Bee, and purpose, a refresh action, a headline block (component split, executions, trails, coverage — no dollar figure), one daily chart with an axis selector, a period breakdown table for the selected axis, a top-trails table, a collapsible daily table, and a warnings area.
3. The chart is a stacked bar chart built with **LayerChart 2**, themed through the library's official DaisyUI 5 stylesheet so that surfaces, text, and marks resolve from `--color-base-*` and semantic theme tokens rather than literals. The library is imported in exactly one in-repo wrapper component (`UsageChart`); pages and stores do not import it directly, so the dependency can be replaced without touching a route.
4. LayerChart is the only charting dependency. It requires Svelte `^5.57.0`, so the console's declared Svelte range is raised to match instead of relying on a lockfile that happens to resolve a compatible version. Series colours come from a fixed ordered palette of theme tokens mapped deterministically from the axis key, so the same model keeps the same colour across reloads and ranges.
5. The number of stacked series is bounded by the palette: the chart shows the top keys for the selected axis and groups the remainder as `other`, while the breakdown table lists every key. A legend that repeats itself for twenty models is not a legend.
6. Unmeasured and incomplete days are distinguished by more than colour (label or pattern), and the chart carries a text summary for assistive technology. The daily table is the authoritative reading of the same data.
7. The selected axis, range, and filters live in the URL, written with the Console's existing replace semantics, so a view is shareable and Back does not walk through keystrokes.
8. The page performs no polling. It reads once on mount and on an explicit refresh or range/filter change. This is a deliberate departure from the live routes and follows from an endpoint that walks run directories.
9. Trail links from the top-trails table open the existing trail detail, which already shows that trail's usage; the aggregate page does not re-implement trail detail.
10. Clicking a day column in the chart or a row in the breakdown table narrows the current view instead of navigating: a day selection restricts the range to that day, and a row adds its key to the filters. The narrowing appears as removable chips and in the URL, so it is reversible without Back, and the operator keeps the context they narrowed from. A trail link is the one click that leaves the page.
11. Empty, partial, and error states are distinct: no executions in range, executions without reported usage, a range older than the retained record, a range longer than the cap, and a failed read each say what they are. The last good report stays on screen when a refresh fails.
12. The design-system document gains a chart section — the chosen library, the one import that themes it, the wrapper rule, the palette order, the bounded-series rule, and the accessibility contract — because the component inventory is part of that contract.

### 7. CLI

1. `paseka inspect usage` keeps `--trace` and `--agent` behaviour. `--trace` becomes optional; without it, the range mode applies with the same defaults as the API (last 30 days ending now), and `--since`, `--until`, `--timezone`, `--adapter`, `--model`, `--bee`, `--purpose`, `--group-by`, and `--json` apply. The text output always states the resolved range, so the default is never silent.
2. The text form prints period totals, one row per day with components and coverage, and the period ranking for the requested axis with shares. `--json` prints exactly the report the API returns.
3. The CLI is the reference reader: its output is the fixture the Console contract tests compare against, so the two surfaces cannot drift into different definitions of a day or a total.

### 8. Naming

1. The surface is called **Usage**, matching `paseka inspect usage` and the protocol's `usage` object. It is not called "billing", "spend", or "cost": those name money, and this report is token volume with an optional recorded cost.
2. Honey is not part of this report. Honey is a separate reserve with its own ledger semantics ([043b](043b-trail-metrics-and-work-completion.md), [044b](044b-standing-supersession-and-energy-observations.md)); relating the two is a separate design ([Backlog](../plans/backlog.md)).

## Testing Decisions

Tests assert external behaviour: a report built from fixture run directories, a JSON contract, CLI output, and a rendered page. No model calls are involved.

- **Aggregation:** fixtures cover a completed AFK run with usage, one without usage, an in-flight run, an interactive session whose usage is in `session.json`, a directory carrying both records, a resumed session, a run with an unknown model, a legacy run without a purpose, and a run with no start time. Each day's axis arrays sum to that day's total; period totals equal the sum of days; a run crossing midnight appears on exactly one day.
- **Boundaries:** `since` inclusive and `until` exclusive; zero-filled days; empty days versus unmeasured days; a daylight-saving transition in a named zone; a run exactly at midnight; an invalid zone, a reversed range, an over-long range, and an unparsable date each fail with a clear error.
- **Coverage:** absent usage yields `missing`, never zero tokens; an in-flight run yields `unknown` and marks the day incomplete; a range older than the retained record warns; an unreadable run directory warns instead of disappearing; a pruned trail is absent from the report rather than reported as zero.
- **Deduplication:** a directory with two usage records contributes once; two run directories for a resumed session contribute twice.
- **Contract:** the API response's field set, day count, and echoed timezone match the request; the response contains no price or estimate field; a read changes no file and publishes no event.
- **Console:** the route is registered with a unique chord and glyph; the axis selector changes the stack without a second fetch; the URL round-trips range and filters; clicking a day or a breakdown row narrows the view through the URL with removable chips and does not navigate away; the headline contains no cost figure; the empty, partial, and error states render their distinct copy; the daily table matches the chart series; the page does not start a poller.
- **Design system:** no raw colour literals in the new components; every series colour resolves from a theme token; switching DaisyUI themes repaints the chart and its tooltip without a reload and without re-fetching; the chart legend is present; the summary text is available to assistive technology; the production build reports the dependency's real added cost, which is reviewed rather than assumed.
- **CLI:** text output is stable for a fixture; `--json` equals the API report for the same query; `--trace` behaviour is unchanged.

Prior art: run and trace-scan projection tests, `inspect usage` and `inspect metrics` output tests, console handler contract tests, trail-metrics coverage tests from [043b](043b-trail-metrics-and-work-completion.md), and the Svelte route and store tests with the shared console fixtures.

## Out of Scope

- **Price tables, rate cards, currency conversion, cost estimates.** Token counts are recorded; recorded provider cost is shown where it exists and nowhere invented.
- **Honey spending and the honey↔token relation.** A separate ledger and a separate design ([Backlog](../plans/backlog.md), *Honey ↔ LLM token billing*).
- **Billing, quotas, budgets, and alerts.** This is an observation, not an enforcement surface.
- **Live per-token telemetry.** Usage arrives at the end of an execution; the page is history with an explicit refresh.
- **Filling the Cursor HITL gap.** Cursor interactive sessions still report no usage; the page counts them as unmeasured ([Backlog](../plans/backlog.md), *Interactive session usage (Cursor)*).
- **A colony-wide metrics database or exported aggregates.** The report is a projection of retained records; nothing is persisted.
- **Cross-colony or machine-level aggregation.** One colony root, one report.
- **Trail detail, per-task cost attribution, and fork/configuration attribution.** They belong to the trail surfaces and the trail metrics.
- **Retention changes.** Pruning keeps its current behaviour; the report only states what retention left.
- **A second charting library or design language.** LayerChart is the only chart dependency; it is themed from DaisyUI tokens. Chart packs that bring their own token vocabulary (shadcn-style copies, Carbon, a library-styled default palette) are not adopted, because a second visual language beside DaisyUI is the one thing the design-system contract forbids.
- **CSV/PNG export of the page.** `--json` is the export path in v1.

## Further Notes

### Resolved product decisions

- **D1 — Recorded cost stays out of the headline.** An adapter-dependent dollar sum is shown in breakdown rows and the daily table with its own reporting count, and never as the first number on the page. Tokens remain the headline because they are the measurement every reporting adapter provides; a partial cost promoted to the top would read as a colony bill.
- **D2 — Default range is 30 days.** Seven and ninety remain one click away, and a standing-trail heavy colony can still pick its own range, but the first view answers "this month".
- **D3 — Bee is kept as both an axis and a filter.** The field is already recorded per execution, and one expensive role hiding inside a colony total is exactly the kind of question this page exists to answer. Demoting it later is a UI change, not a contract change.
- **D4 — Drill-down filters in place.** Clicking a day or a breakdown row narrows the current view — the day becomes the range, the row becomes a filter — with removable chips and the URL recording the narrowing. Only a trail link leaves the page; the rest keeps the operator in the context they were reading.

### Chart library

LayerChart 2 was chosen over both in-repo SVG primitives and the other Svelte-capable libraries for one reason that outweighs the rest: its official DaisyUI 5 stylesheet maps the library's surface variables onto `--color-base-*`, so a chart is themed by the same `data-theme` switch as the rest of the console, with no theme object, no colour table, and no re-render on theme change. It is Svelte 5 native (`Chart` + marks), gives stacking, axes, grid, legend, tooltip, highlight, and a click callback for the day/row filter without hand-building them, and is actively released (2.10.x, November 2026).

Alternatives and why they lost:

- **In-repo SVG primitives.** Zero dependency, perfect control, but we would own scales, stacking, tooltips, legend, responsiveness, and their tests for one chart type. Reasonable only if charts never grow beyond this page.
- **Unovis.** Modular and actively maintained, but the Svelte wrapper is configuration objects rather than composable markup, and its `--vis-*` variables need a hand-written bridge to DaisyUI.
- **ECharts 6.** Powerful, but a large tree-shaken payload, an imperative option object, a separate theme JSON, and a stale Svelte wrapper pinned to ECharts 5.
- **Chart.js 4.** Canvas cannot inherit CSS variables: colours must be resolved at runtime and re-applied on every theme switch, and accessibility needs a parallel fallback.
- **Observable Plot.** Pleasant defaults, but it renders into a container rather than composing with Svelte, and its own typography and palette fight the design system.
- **shadcn-svelte charts / Carbon.** Copy-paste or prebuilt components typed to a different token vocabulary; they would introduce a second design language beside DaisyUI.

Two risks are accepted and contained rather than ignored. LayerChart 2 requires Svelte `^5.57.0`, which raises the console's declared range. Its 2.x line moves quickly and pulls first-party `@layerstack/*` pre-release packages, which is why use is confined to one wrapper component and reviewed before a minor upgrade. Bundle cost is checked by the frontend build rather than assumed, since tree-shaking only pays for the components actually imported.

### Why not a metrics daemon

The data already exists per execution. A collector would have to re-derive it, keep its own store, and go stale when the runtime's interpretation changes. The reader stays in Go beside the trail metrics, uses the same projection, and re-derives on demand; only a short-lived in-process cache exists, and it is allowed to be wrong about age only by being explicit about `observedAt`.

### Relationship to other specs

[043b](043b-trail-metrics-and-work-completion.md) deliberately left statistics over many trails to a reader and recorded the vocabulary this report reuses: execution purpose, one-source usage precedence, and coverage instead of zero. [043a](043a-truthful-run-records.md) is why a run's usage record can be trusted as the outcome of one execution rather than a mixture of two. [037](037-opencode-v2-proxy.md) and [019](019-model-aliases.md) are why adapter and model are two labels rather than one. [051](051-console-root-cutover.md) may change where the route is mounted; the route name and the API path are unaffected.

### Reading the chart honestly

Token volume is not value. A model that replays a large cached context can show millions of tokens at a small cost, because cache reads are billed far below input, while a short uncached run can cost more with a fraction of the volume. That is why the default stack shows components, why cache kinds are never folded into input, and why the page says "tokens" and not "cost".
