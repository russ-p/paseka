import { fetchUsage as requestUsage } from '$lib/api/client';
import type { UsageReport } from '$lib/api/types';
import {
	usageErrorKind,
	usageRangeTooLong,
	usageTooLongMessage,
	type UsageReadErrorKind
} from '$lib/usage-honesty';
import {
	browserTimezone,
	clearUsageFilters,
	hasUsageFilters,
	readUsageView,
	toggleUsageFilter,
	usageCustomRange,
	usageDayRange,
	usageFilterCount,
	usagePresetRange,
	withUsageFilter,
	withoutUsageFilter,
	writeUsageView,
	type UsageAxisChoice,
	type UsageFilterAxis,
	type UsageFilters,
	type UsagePreset,
	type UsageRangeChoice,
	type UsageViewDefaults
} from '$lib/usage-view';

export {
	browserTimezone,
	inclusiveEnd,
	localDate,
	parseLocalDate,
	usageCustomRange,
	usageDayRange,
	usagePresetRange,
	type UsageAxisChoice,
	type UsageFilterAxis,
	type UsageFilters,
	type UsagePreset,
	type UsageRangeChoice
} from '$lib/usage-view';

/** The query one read sends: the range, the zone, the filter set, and whether
 * the read is the explicit refresh that bypasses the reader's scan cache. */
export type UsageFetchQuery = {
	since: string;
	until: string;
	timezone: string;
	refresh?: boolean;
} & UsageFilters;

interface UsageStoreOptions {
	fetchUsage?: (query: UsageFetchQuery) => Promise<UsageReport>;
	/** Injectable clock, so a test's preset range is fixed rather than "today". */
	now?: () => Date;
	/** Injectable zone; the page uses the browser's. */
	timezone?: string;
	/** The preset a view falls back to when the URL names no range. */
	initialPreset?: UsagePreset;
	/** The axis a view falls back to when the URL names none. */
	initialAxis?: UsageAxisChoice;
	/** The filters a view falls back to when the URL carries none. */
	initialFilters?: Partial<UsageFilters>;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

/**
 * The Usage page's state.
 *
 * This is a deliberate departure from the live routes: the report walks run
 * directories, so there is no timer here. It reads once on mount, and again on a
 * range or filter change or an explicit refresh. Switching the breakdown axis
 * reads nothing — all four axes are already in the report. A failed read keeps
 * the last good report on screen, because the numbers an operator is reading are
 * still the last real ones.
 *
 * The server may answer from its short-lived scan cache; the report's
 * `observedAt` says which scan produced the numbers, so a reused answer is
 * explicit rather than hidden. `refresh()` is the one read that asks for a new
 * scan, so a trail that finished since the last read appears on demand.
 *
 * The view lives in the URL: the store seeds itself from the address bar and
 * publishes each change with `replaceState`, so reload and a shared link restore
 * the same view and Back does not step through the values the operator added.
 *
 * Drill-down is a second narrowing on top of the chosen range: a day is kept
 * beside the range rather than replacing it, so the range the operator was
 * reading is one chip removal away. Clicking the same day again, or removing the
 * day chip, restores that range without a Back press.
 */
export function createUsageStore(options: UsageStoreOptions = {}) {
	const readUsage = options.fetchUsage ?? requestUsage;
	const clock = options.now ?? (() => new Date());
	const zone = options.timezone ?? browserTimezone();

	const defaults: UsageViewDefaults = {
		preset: options.initialPreset ?? 30,
		axis: options.initialAxis,
		filters: options.initialFilters
	};
	const initial = readUsageView(clock(), defaults);

	let preset = $state(initial.preset);
	let since = $state(initial.since);
	let until = $state(initial.until);
	let day = $state<string | null>(initial.day);
	let axis = $state<UsageAxisChoice>(initial.axis);
	let filters = $state<UsageFilters>(initial.filters);
	let report = $state<UsageReport | null>(null);
	let loading = $state(true);
	let error = $state('');
	// What the last failure was. A range over the cap is refused before it is
	// sent; any other failure is a read the page could not complete. The page
	// gives each its own copy rather than showing one generic error for both.
	let errorKind = $state<UsageReadErrorKind | null>(null);

	let started = false;
	let generation = 0;

	function publish(): void {
		writeUsageView({ since, until, day, axis, filters });
	}

	/** The range one read sends: the drilled day when there is one. */
	function queryRange(): { since: string; until: string } {
		if (!day) return { since, until };
		return usageDayRange(day) ?? { since, until };
	}

	async function load(refresh = false): Promise<void> {
		const request = ++generation;
		const range = queryRange();
		// The page refuses a range the reader would refuse, before walking run
		// directories for it. The last good report stays on screen; only the
		// state changes.
		if (usageRangeTooLong(range.since, range.until)) {
			error = usageTooLongMessage();
			errorKind = 'range-too-long';
			loading = false;
			return;
		}
		loading = true;
		try {
			const next = await readUsage({
				since: range.since,
				until: range.until,
				timezone: zone,
				adapter: filters.adapter,
				model: filters.model,
				bee: filters.bee,
				purpose: filters.purpose,
				// Only the explicit refresh asks for a new scan; a range or
				// filter read may reuse the report the server already parsed.
				...(refresh ? { refresh: true } : {})
			});
			// A slow read for a view the operator already left must not replace
			// the answer they are waiting for.
			if (request !== generation) return;
			report = next;
			error = '';
			errorKind = null;
		} catch (cause) {
			if (request !== generation) return;
			error = errorMessage(cause);
			// The reader's own refusal (a range over the cap) is not a failed
			// read, and the page says so with different copy.
			errorKind = usageErrorKind(cause);
		} finally {
			if (request === generation) loading = false;
		}
	}

	/**
	 * The refresh control: an explicit re-scan. It bypasses the server's
	 * short-lived cache, so a run that finished since the last read appears
	 * without a reload, and the returned report carries a new `observedAt`.
	 */
	function refresh(): Promise<void> {
		return load(true);
	}

	function applyPreset(days: UsagePreset): void {
		const next = usagePresetRange(days, clock());
		preset = days;
		since = next.since;
		until = next.until;
		// Choosing a range leaves any drill-down: the new range is the view.
		day = null;
		publish();
		void load();
	}

	/** Applies a custom range; false means the chosen dates are not a range. */
	function applyCustom(from: string, to: string): boolean {
		const next = usageCustomRange(from, to);
		if (!next) return false;
		preset = 'custom';
		since = next.since;
		until = next.until;
		day = null;
		publish();
		void load();
		return true;
	}

	/**
	 * Narrows the view to one local day, keeping the range, axis, and filters.
	 * Asking for the day already in view undoes the narrowing, so the chart's
	 * day control is a toggle and one control can reverse it.
	 */
	function selectDay(date: string): void {
		if (date === day) {
			clearDay();
			return;
		}
		if (!usageDayRange(date)) return;
		day = date;
		publish();
		void load();
	}

	/** Drops the drill-down and reads the range it was narrowed from. */
	function clearDay(): void {
		if (day === null) return;
		day = null;
		publish();
		void load();
	}

	/** Changes the chart stack; all four axes are already loaded, so no read. */
	function setAxis(next: UsageAxisChoice): void {
		if (axis === next) return;
		axis = next;
		publish();
	}

	/** Adds one filter value; the same value twice is one value and no read. */
	function addFilter(axisName: UsageFilterAxis, value: string): void {
		const next = withUsageFilter(filters, axisName, value);
		if (next === filters) return;
		filters = next;
		publish();
		void load();
	}

	/**
	 * Adds a value the view does not filter by yet, or removes one it does: the
	 * breakdown row's click, which is one control in two directions.
	 */
	function toggleFilter(axisName: UsageFilterAxis, value: string): void {
		const next = toggleUsageFilter(filters, axisName, value);
		if (next === filters) return;
		filters = next;
		publish();
		void load();
	}

	function removeFilter(axisName: UsageFilterAxis, value: string): void {
		const next = withoutUsageFilter(filters, axisName, value);
		if (next === filters) return;
		filters = next;
		publish();
		void load();
	}

	function clearFilters(): void {
		const next = clearUsageFilters(filters);
		if (next === filters) return;
		filters = next;
		publish();
		void load();
	}

	function start(): void {
		if (started) return;
		started = true;
		// The address bar is made exact before the first read, so a copied link
		// names the days on screen rather than the default of the day it opens.
		publish();
		void load();
	}

	/** No timer exists, so stopping only drops the mount latch. */
	function stop(): void {
		started = false;
	}

	return {
		get report(): UsageReport | null {
			return report;
		},
		get preset(): UsageRangeChoice {
			return preset;
		},
		get since(): string {
			return since;
		},
		get until(): string {
			return until;
		},
		/** The drill-down day in view, or `null` for the whole range. */
		get day(): string | null {
			return day;
		},
		get timezone(): string {
			return zone;
		},
		get axis(): UsageAxisChoice {
			return axis;
		},
		get filters(): UsageFilters {
			return filters;
		},
		get hasFilters(): boolean {
			return hasUsageFilters(filters);
		},
		get filterCount(): number {
			return usageFilterCount(filters);
		},
		get loading(): boolean {
			return loading;
		},
		/** Skeletons only before the first payload; a later failure keeps it. */
		get showSkeletons(): boolean {
			return loading && report === null;
		},
		get lastError(): string {
			return error;
		},
		/**
		 * Why the last read failed, or `null` when the report on screen is the
		 * answer to the current view. `range-too-long` is a bound the page
		 * refuses before sending; `read-failed` is a read that did not complete.
		 */
		get lastErrorKind(): UsageReadErrorKind | null {
			return errorKind;
		},
		applyPreset,
		applyCustom,
		selectDay,
		clearDay,
		setAxis,
		addFilter,
		toggleFilter,
		removeFilter,
		clearFilters,
		refresh,
		start,
		stop
	};
}

export type UsageStore = ReturnType<typeof createUsageStore>;
