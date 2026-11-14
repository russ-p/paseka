import { describe, expect, it } from 'vitest';
import {
	USAGE_MAX_DAYS,
	usageCoverageGaps,
	usageErrorKind,
	usageOtherWarnings,
	usagePredatesRetained,
	usageRangeDays,
	usageRangeTooLong,
	usageRetainedWarning
} from './usage-honesty';
import { usageReport, usageDay } from '../tests/fixtures';

describe('usage range bounds', () => {
	it('counts whole local days in an inclusive/exclusive range', () => {
		expect(usageRangeDays('2026-09-01', '2026-09-02')).toBe(1);
		expect(usageRangeDays('2026-09-01', '2026-10-01')).toBe(30);
		// A daylight-saving fall-back still names whole dates, not 23-hour days.
		expect(usageRangeDays('2026-10-24', '2026-10-31')).toBe(7);
		expect(usageRangeDays('2026-09-02', '2026-09-01')).toBeNull();
		expect(usageRangeDays('2026-09-01', '2026-09-01')).toBeNull();
		expect(usageRangeDays('not-a-date', '2026-09-01')).toBeNull();
	});

	it('refuses only a range longer than the cap', () => {
		expect(usageRangeTooLong('2026-01-01', '2026-01-02')).toBe(false);
		// Exactly the cap is accepted; one day more is not.
		expect(usageRangeTooLong('2025-01-01', '2026-01-02')).toBe(false);
		expect(usageRangeTooLong('2025-01-01', '2026-01-03')).toBe(true);
		expect(usageRangeDays('2024-01-01', '2026-12-31')).toBeGreaterThan(USAGE_MAX_DAYS);
	});
});

describe('usage read failures', () => {
	it('separates the range cap from a failed read', () => {
		expect(usageErrorKind(new Error('range is longer than 366 days'))).toBe('range-too-long');
		expect(usageErrorKind(new Error('until 2026-10-01 must be after since 2026-10-02'))).toBe(
			'read-failed'
		);
		expect(usageErrorKind('server is busy')).toBe('read-failed');
	});
});

describe('usage coverage gaps', () => {
	it('reads the excluded-undated count and the unreadable warnings from the report', () => {
		const report = usageReport({
			excludedUndated: 3,
			warnings: [
				{ group: 'runs', kind: 'unreadable', detail: 'run trace-a/run-1: no request record' },
				{ group: 'runs', kind: 'unreadable', detail: 'run trace-b/run-2: bad json' },
				{ group: 'range', kind: 'missing', detail: '3 executions have no usable start time' }
			]
		});
		expect(usageCoverageGaps(report)).toEqual({ undated: 3, unreadable: 2 });
	});

	it('reports no gaps when the reader had none', () => {
		expect(usageCoverageGaps(usageReport())).toEqual({ undated: 0, unreadable: 0 });
	});
});

describe('usage retained-record boundary', () => {
	const predates = usageReport({
		days: [usageDay({ date: '2026-09-01', executions: 0, reportedExecutions: 0 })],
		warnings: [
			{
				group: 'coverage',
				kind: 'missing',
				detail:
					'range begins before the earliest retained run (2026-09-10T08:00:00Z); days before it are unmeasured, not quiet'
			}
		]
	});

	it('takes the warning as the truth rather than comparing dates itself', () => {
		expect(usagePredatesRetained(predates)).toBe(true);
		expect(usageRetainedWarning(predates)?.group).toBe('coverage');
		expect(usagePredatesRetained(usageReport())).toBe(false);
	});

	it('keeps the boundary out of the generic warning list', () => {
		const report = usageReport({
			warnings: [...predates.warnings, { group: 'range', kind: 'missing', detail: 'undated' }]
		});
		expect(usageOtherWarnings(report).map((warning) => warning.group)).toEqual(['range']);
	});
});
