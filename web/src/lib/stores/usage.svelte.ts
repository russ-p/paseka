import { fetchUsage as requestUsage } from '$lib/api/client';
import type { UsageReport } from '$lib/api/types';
import {
	browserTimezone,
	clearUsageFilters,
	hasUsageFilters,
	readUsageView,
	usageCustomRange,
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
	usagePresetRange,
	type UsageAxisChoice,
	type UsageFilterAxis,
	type UsageFilters,
	type UsagePreset,
	type UsageRangeChoice
} from '$lib/usage-view';

/** The query one read sends: the range, the zone, and the filter set. */
export type UsageFetchQuery = {
	since: string;
	until: string;
	timezone: string;
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
 * The view lives in the URL: the store seeds itself from the address bar and
 * publishes each change with `replaceState`, so reload and a shared link restore
 * the same view and Back does not step through the values the operator added.
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
	let axis = $state<UsageAxisChoice>(initial.axis);
	let filters = $state<UsageFilters>(initial.filters);
	let report = $state<UsageReport | null>(null);
	let loading = $state(true);
	let error = $state('');

	let started = false;
	let generation = 0;

	function publish(): void {
		writeUsageView({ since, until, axis, filters });
	}

	async function load(): Promise<void> {
		const request = ++generation;
		loading = true;
		try {
			const next = await readUsage({
				since,
				until,
				timezone: zone,
				adapter: filters.adapter,
				model: filters.model,
				bee: filters.bee,
				purpose: filters.purpose
			});
			// A slow read for a view the operator already left must not replace
			// the answer they are waiting for.
			if (request !== generation) return;
			report = next;
			error = '';
		} catch (cause) {
			if (request !== generation) return;
			error = errorMessage(cause);
		} finally {
			if (request === generation) loading = false;
		}
	}

	function applyPreset(days: UsagePreset): void {
		const next = usagePresetRange(days, clock());
		preset = days;
		since = next.since;
		until = next.until;
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
		publish();
		void load();
		return true;
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
		applyPreset,
		applyCustom,
		setAxis,
		addFilter,
		removeFilter,
		clearFilters,
		refresh: load,
		start,
		stop
	};
}

export type UsageStore = ReturnType<typeof createUsageStore>;
