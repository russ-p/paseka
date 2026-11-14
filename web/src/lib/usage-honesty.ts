/**
 * The honesty rules the Usage page applies to a report it already holds.
 *
 * Every total on the page is a total over what the reader could see, so these
 * helpers name what it could not: a range that reaches past the retained record,
 * edges the reader excluded, and the one range the reader refuses outright.
 * Nothing here reads usage; the page and the chart both work from the report
 * `GET /api/usage` returned.
 */
import type { UsageReport, UsageWarning } from '$lib/api/types';
import { parseLocalDate } from '$lib/usage-view';

/**
 * The longest range the report builds, mirroring `runs.UsageMaxDays`. A page
 * that guessed a different cap would send a request the reader refuses, so the
 * page refuses it first and says which bound it hit.
 */
export const USAGE_MAX_DAYS = 366;

/** The warning group the reader uses for the retained-record boundary. */
export const USAGE_COVERAGE_WARNING = 'coverage';

/** The warning group and kind an unreadable run record carries. */
export const USAGE_RUNS_WARNING = 'runs';
export const USAGE_UNREADABLE_WARNING = 'unreadable';

/**
 * Whole local days in an inclusive `since` / exclusive `until` pair, or `null`
 * when the pair is not a calendar range. Counting by calendar increments keeps
 * the answer correct across a daylight-saving transition. The count stops one
 * day past the cap, because a caller only needs to know that the range is too
 * long, not how many years it names.
 */
export function usageRangeDays(since: string, until: string): number | null {
	const start = parseLocalDate(since);
	const end = parseLocalDate(until);
	if (!start || !end || end.getTime() <= start.getTime()) return null;
	let days = 0;
	for (const cursor = new Date(start); cursor < end; cursor.setDate(cursor.getDate() + 1)) {
		days += 1;
		if (days > USAGE_MAX_DAYS) return days;
	}
	return days;
}

/** True when the range is longer than the report's cap. */
export function usageRangeTooLong(since: string, until: string): boolean {
	const days = usageRangeDays(since, until);
	return days !== null && days > USAGE_MAX_DAYS;
}

/** The sentence a range over the cap carries, so both surfaces say the same thing. */
export function usageTooLongMessage(): string {
	return `This range is longer than the report's ${USAGE_MAX_DAYS}-day cap. Choose a shorter range.`;
}

/**
 * What a failed read was: a range the reader refuses by design, or a read that
 * did not happen. The two are different states on the page — one is a bound the
 * operator crossed, the other is a failure to keep the last good report beside.
 */
export type UsageReadErrorKind = 'range-too-long' | 'read-failed';

/**
 * Classifies a read failure. The client refuses an over-long range before it is
 * sent, so this is the fallback for a reader or proxy that answered instead; the
 * server's plain-text 400 is the contract, and both of its phrasings name the
 * cap in days.
 */
export function usageErrorKind(error: unknown): UsageReadErrorKind {
	const message = error instanceof Error ? error.message : String(error);
	return /longer than \d+ days/i.test(message) ? 'range-too-long' : 'read-failed';
}

/** The edges a report says it could not place or open. */
export interface UsageCoverageGaps {
	/** Executions retained on disk with no usable start time. */
	undated: number;
	/** Run records the reader could not open. */
	unreadable: number;
}

/**
 * The coverage gaps of one report. Both counts are read from the report itself:
 * the excluded-undated total it carries, and the unreadable-run warnings it
 * names. A gap is never subtracted from a total silently — the page repeats
 * these beside the coverage number.
 */
export function usageCoverageGaps(report: UsageReport): UsageCoverageGaps {
	return {
		undated: report.excludedUndated,
		unreadable: report.warnings.filter(
			(warning) =>
				warning.group === USAGE_RUNS_WARNING && warning.kind === USAGE_UNREADABLE_WARNING
		).length
	};
}

/** The warning that says the range reaches past the retained record, if any. */
export function usageRetainedWarning(report: UsageReport): UsageWarning | undefined {
	return report.warnings.find((warning) => warning.group === USAGE_COVERAGE_WARNING);
}

/**
 * True when the range begins before the retained record. The reader decides
 * this, not the page: a page that compared dates itself could disagree with the
 * warning the CLI prints for the same range.
 */
export function usagePredatesRetained(report: UsageReport): boolean {
	return usageRetainedWarning(report) !== undefined;
}

/**
 * The warnings that are not the retained-record boundary. That one has its own
 * state, so repeating its detail in the generic list would say the same thing
 * twice.
 */
export function usageOtherWarnings(report: UsageReport): UsageWarning[] {
	return report.warnings.filter((warning) => warning.group !== USAGE_COVERAGE_WARNING);
}
