import { fetchUsage as requestUsage } from '$lib/api/client';
import type { UsageReport } from '$lib/api/types';

/** The range presets the page offers. Custom is a separate choice. */
export type UsagePreset = 7 | 30 | 90;
export type UsageRangeChoice = UsagePreset | 'custom';

interface UsageStoreOptions {
	fetchUsage?: (query: { since: string; until: string; timezone: string }) => Promise<UsageReport>;
	/** Injectable clock, so a test's preset range is fixed rather than "today". */
	now?: () => Date;
	/** Injectable zone; the page uses the browser's. */
	timezone?: string;
	/** The first range, matching the server's own 30-day default. */
	initialPreset?: UsagePreset;
}

function errorMessage(error: unknown): string {
	return error instanceof Error ? error.message : String(error);
}

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

/**
 * The Usage page's state.
 *
 * This is a deliberate departure from the live routes: the report walks run
 * directories, so there is no timer here. It reads once on mount, and again on a
 * range change or an explicit refresh. A failed read keeps the last good report
 * on screen, because the numbers an operator is reading are still the last real
 * ones.
 */
export function createUsageStore(options: UsageStoreOptions = {}) {
	const readUsage = options.fetchUsage ?? requestUsage;
	const clock = options.now ?? (() => new Date());
	const zone = options.timezone ?? browserTimezone();

	const initial = usagePresetRange(options.initialPreset ?? 30, clock());

	let preset = $state<UsageRangeChoice>(options.initialPreset ?? 30);
	let since = $state(initial.since);
	let until = $state(initial.until);
	let report = $state<UsageReport | null>(null);
	let loading = $state(true);
	let error = $state('');

	let started = false;
	let generation = 0;

	async function load(): Promise<void> {
		const request = ++generation;
		loading = true;
		try {
			const next = await readUsage({ since, until, timezone: zone });
			// A slow read for a range the operator already left must not replace
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
		void load();
	}

	/** Applies a custom range; false means the chosen dates are not a range. */
	function applyCustom(from: string, to: string): boolean {
		const next = usageCustomRange(from, to);
		if (!next) return false;
		preset = 'custom';
		since = next.since;
		until = next.until;
		void load();
		return true;
	}

	function start(): void {
		if (started) return;
		started = true;
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
		refresh: load,
		start,
		stop
	};
}

export type UsageStore = ReturnType<typeof createUsageStore>;
