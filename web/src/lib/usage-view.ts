/**
 * The Usage page's view: its range, its axis, and its filters, as the URL
 * carries them.
 *
 * A Usage view is a link. `since` and `until` are the same local calendar dates
 * the API takes, `axis` names the breakdown the chart stacks, and each filter
 * axis is a repeated param (`?model=a&model=b`). The defaults spend no query for
 * the axis, so an ordinary view stays a short address.
 *
 * Writes use `replaceState`, never `pushState`: changing a filter is not
 * navigation, and Back must not walk an operator backwards through the values
 * they added. It also means the page never has to react to a history move,
 * because nothing it does creates one the operator could move between.
 */
import { untrack } from 'svelte';
import { replaceState } from '$app/navigation';
import { page } from '$app/state';
import type { UsageReport } from '$lib/api/types';

/** The range presets the page offers. Custom is a separate choice. */
export type UsagePreset = 7 | 30 | 90;
export type UsageRangeChoice = UsagePreset | 'custom';

/** The four axes a report breaks down and a caller can filter by. */
export const USAGE_FILTER_AXES = ['adapter', 'model', 'bee', 'purpose'] as const;
export type UsageFilterAxis = (typeof USAGE_FILTER_AXES)[number];

/** What the chart stacks: the token components or one breakdown axis. */
export type UsageAxisChoice = 'components' | UsageFilterAxis;

/** The repeated filter values of one view, one list per axis. */
export interface UsageFilters {
	adapter: string[];
	model: string[];
	bee: string[];
	purpose: string[];
}

/** Everything the URL carries: dates, the selected axis, and the filters. */
export interface UsageViewState {
	since: string;
	until: string;
	axis: UsageAxisChoice;
	filters: UsageFilters;
}

/** The view plus how its range is named in the control. */
export interface UsageView extends UsageViewState {
	preset: UsageRangeChoice;
}

/** Initial state for a store whose URL carries nothing yet. */
export interface UsageViewDefaults {
	preset?: UsagePreset;
	axis?: UsageAxisChoice;
	filters?: Partial<UsageFilters>;
}

export const usagePresets: UsagePreset[] = [7, 30, 90];

/** A local calendar date as `YYYY-MM-DD`, never shifted through UTC. */
export function localDate(date: Date): string {
	const year = date.getFullYear();
	const month = String(date.getMonth() + 1).padStart(2, '0');
	const day = String(date.getDate()).padStart(2, '0');
	return `${year}-${month}-${day}`;
}

/** Parses `YYYY-MM-DD` as local midnight, or returns null. */
export function parseLocalDate(value: string): Date | null {
	const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim());
	if (!match) return null;
	const year = Number(match[1]);
	const month = Number(match[2]);
	const day = Number(match[3]);
	const date = new Date(year, month - 1, day);
	if (
		date.getFullYear() !== year ||
		date.getMonth() !== month - 1 ||
		date.getDate() !== day
	) {
		return null;
	}
	return date;
}

/** The inclusive date before an exclusive `until` boundary. */
export function inclusiveEnd(until: string): string {
	const date = parseLocalDate(until);
	if (!date) return until;
	const previous = new Date(date);
	previous.setDate(previous.getDate() - 1);
	return localDate(previous);
}

/**
 * A preset range ending today: `days` local calendar days, with `until` the
 * exclusive midnight after today. Counting whole days is what keeps the range
 * identical across a daylight-saving transition.
 */
export function usagePresetRange(
	days: UsagePreset,
	now: Date
): { since: string; until: string } {
	const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
	const since = new Date(today);
	since.setDate(since.getDate() - (days - 1));
	const until = new Date(today);
	until.setDate(until.getDate() + 1);
	return { since: localDate(since), until: localDate(until) };
}

/**
 * A custom range whose `to` date is inclusive, as the control says. The server
 * takes `until` as exclusive, so the day after `to` is what is sent.
 */
export function usageCustomRange(
	from: string,
	to: string
): { since: string; until: string } | null {
	const start = parseLocalDate(from);
	const end = parseLocalDate(to);
	if (!start || !end || end.getTime() < start.getTime()) return null;
	const until = new Date(end);
	until.setDate(until.getDate() + 1);
	return { since: localDate(start), until: localDate(until) };
}

/** The browser's IANA zone, or UTC when the runtime cannot name one. */
export function browserTimezone(): string {
	try {
		return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
	} catch {
		return 'UTC';
	}
}

/** An empty filter set: the view of every retained record. */
export function emptyUsageFilters(): UsageFilters {
	return { adapter: [], model: [], bee: [], purpose: [] };
}

/** True when at least one axis restricts the report. */
export function hasUsageFilters(filters: UsageFilters): boolean {
	return USAGE_FILTER_AXES.some((axis) => filters[axis].length > 0);
}

/** How many values the view filters by, across all axes. */
export function usageFilterCount(filters: UsageFilters): number {
	return USAGE_FILTER_AXES.reduce((total, axis) => total + filters[axis].length, 0);
}

/**
 * Adds one value to an axis. The current object is returned unchanged when the
 * value is blank or already present, so a click that changes nothing cannot
 * trigger a read or a URL write.
 */
export function withUsageFilter(
	filters: UsageFilters,
	axis: UsageFilterAxis,
	value: string
): UsageFilters {
	const trimmed = value.trim();
	if (trimmed === '' || filters[axis].includes(trimmed)) return filters;
	return { ...filters, [axis]: [...filters[axis], trimmed] };
}

/** Removes one value from an axis, returning the same object when it is absent. */
export function withoutUsageFilter(
	filters: UsageFilters,
	axis: UsageFilterAxis,
	value: string
): UsageFilters {
	if (!filters[axis].includes(value)) return filters;
	return { ...filters, [axis]: filters[axis].filter((entry) => entry !== value) };
}

/** Removes every value, returning the same object when there is nothing to clear. */
export function clearUsageFilters(filters: UsageFilters): UsageFilters {
	return hasUsageFilters(filters) ? emptyUsageFilters() : filters;
}

/**
 * The values the current report offers for one axis. A key appears once even
 * when the axis shows it with more than one secondary label, because the filter
 * is the key: filtering by `opencode` deliberately keeps both implementations.
 * The server's ranking order is kept, so the offered values read in the order of
 * what the view actually spends.
 */
export function usageFilterValues(report: UsageReport, axis: UsageFilterAxis): string[] {
	const seen = new Set<string>();
	const values: string[] = [];
	for (const entry of report.breakdown[axis] ?? []) {
		if (seen.has(entry.key)) continue;
		seen.add(entry.key);
		values.push(entry.key);
	}
	return values;
}

function isUsageAxisChoice(value: string | null): value is UsageAxisChoice {
	return value === 'components' || USAGE_FILTER_AXES.some((axis) => axis === value);
}

function readAxis(params: URLSearchParams, fallback: UsageAxisChoice): UsageAxisChoice {
	const raw = params.get('axis');
	return isUsageAxisChoice(raw) ? raw : fallback;
}

function readFilters(
	params: URLSearchParams,
	axis: UsageFilterAxis,
	fallback: string[]
): string[] {
	// A present param owns the axis, even when it has no values: a cleared filter
	// is an empty list, not an invitation to restore a default.
	if (!params.has(axis)) return [...fallback];
	const values: string[] = [];
	for (const raw of params.getAll(axis)) {
		const value = raw.trim();
		if (value === '' || values.includes(value)) continue;
		values.push(value);
	}
	return values;
}

/** Which preset a range names, if it names one for `now`. */
function detectPreset(
	range: { since: string; until: string },
	now: Date
): UsageRangeChoice | null {
	for (const days of usagePresets) {
		const preset = usagePresetRange(days, now);
		if (preset.since === range.since && preset.until === range.until) return days;
	}
	return null;
}

/**
 * The view the address bar holds, falling back to `defaults` pieces the URL
 * does not carry. A half-written or impossible range is not trusted: an operator
 * editing a link by hand gets the default range rather than a view the server
 * would refuse.
 */
export function readUsageView(now: Date, defaults: UsageViewDefaults = {}): UsageView {
	const fallbackPreset = defaults.preset ?? 30;
	const fallbackFilters = {
		adapter: [...(defaults.filters?.adapter ?? [])],
		model: [...(defaults.filters?.model ?? [])],
		bee: [...(defaults.filters?.bee ?? [])],
		purpose: [...(defaults.filters?.purpose ?? [])]
	};

	if (typeof window === 'undefined') {
		return {
			preset: fallbackPreset,
			...usagePresetRange(fallbackPreset, now),
			axis: defaults.axis ?? 'components',
			filters: fallbackFilters
		};
	}

	const params = new URLSearchParams(window.location.search);
	const since = params.get('since');
	const until = params.get('until');
	const start = since ? parseLocalDate(since) : null;
	const end = until ? parseLocalDate(until) : null;

	let preset: UsageRangeChoice = fallbackPreset;
	let range = usagePresetRange(fallbackPreset, now);
	if (start && end && end.getTime() > start.getTime()) {
		range = { since: since as string, until: until as string };
		preset = detectPreset(range, now) ?? 'custom';
	}

	return {
		preset,
		since: range.since,
		until: range.until,
		axis: readAxis(params, defaults.axis ?? 'components'),
		filters: {
			adapter: readFilters(params, 'adapter', fallbackFilters.adapter),
			model: readFilters(params, 'model', fallbackFilters.model),
			bee: readFilters(params, 'bee', fallbackFilters.bee),
			purpose: readFilters(params, 'purpose', fallbackFilters.purpose)
		}
	};
}

/**
 * Publishes the view. Path and hash are kept, unrelated params are somebody
 * else's, and a write that changes no query costs no history call.
 *
 * The range is written explicitly even when it happens to equal a preset: a
 * shared link names the days it showed, rather than depending on the day it is
 * opened.
 */
export function writeUsageView(view: UsageViewState): void {
	if (typeof window === 'undefined') return;
	const url = new URL(window.location.href);
	url.searchParams.set('since', view.since);
	url.searchParams.set('until', view.until);
	if (view.axis === 'components') url.searchParams.delete('axis');
	else url.searchParams.set('axis', view.axis);

	for (const axis of USAGE_FILTER_AXES) {
		url.searchParams.delete(axis);
		for (const value of view.filters[axis]) url.searchParams.append(axis, value);
	}

	const query = url.search;
	if (query === window.location.search) return;
	replaceState(`${url.pathname}${query}${url.hash}`, untrack(() => page.state));
}
