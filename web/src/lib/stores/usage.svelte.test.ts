import { describe, expect, it, vi } from 'vitest';
import {
	createUsageStore,
	inclusiveEnd,
	usageCustomRange,
	usagePresetRange
} from './usage.svelte';
import { usageReport } from '../../tests/fixtures';
import type { UsageReport } from '$lib/api/types';

type UsageQuery = { since: string; until: string; timezone: string };

function fixedNow(): Date {
	return new Date(2026, 8, 30, 12, 0, 0);
}

describe('usage ranges', () => {
	it('counts whole local days ending today, across a daylight-saving change', () => {
		// Berlin falls back on 2026-10-25; the range still names whole dates.
		expect(usagePresetRange(7, new Date(2026, 9, 30, 9, 0, 0))).toEqual({
			since: '2026-10-24',
			until: '2026-10-31'
		});
		expect(usagePresetRange(30, fixedNow())).toEqual({
			since: '2026-09-01',
			until: '2026-10-01'
		});
	});

	it('treats a custom range end as inclusive and sends the following midnight', () => {
		expect(usageCustomRange('2026-09-01', '2026-09-07')).toEqual({
			since: '2026-09-01',
			until: '2026-09-08'
		});
		expect(usageCustomRange('2026-09-07', '2026-09-01')).toBeNull();
		expect(usageCustomRange('not-a-date', '2026-09-07')).toBeNull();
		expect(inclusiveEnd('2026-09-08')).toBe('2026-09-07');
	});
});

describe('createUsageStore', () => {
	it('ignores a slow read for a range the operator already left', async () => {
		let resolveFirst: ((report: UsageReport) => void) | undefined;
		const fetchUsage = vi
			.fn<(query: UsageQuery) => Promise<UsageReport>>()
			.mockImplementationOnce(
				() =>
					new Promise<UsageReport>((resolve) => {
						resolveFirst = resolve;
					})
			)
			.mockResolvedValueOnce(usageReport());
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });

		const first = store.refresh();
		store.applyPreset(7);
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		resolveFirst?.(usageReport());
		await first;

		// The 7-day answer is the one on screen; the abandoned 30-day read cannot
		// replace it when it lands late.
		expect(store.preset).toBe(7);
		expect(store.since).toBe('2026-09-24');
	});

	it('keeps the last good report when a refresh fails', async () => {
		const good = usageReport();
		const fetchUsage = vi
			.fn<() => Promise<UsageReport>>()
			.mockResolvedValueOnce(good)
			.mockRejectedValueOnce(new Error('server is busy'));
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });

		await store.refresh();
		expect(store.report).toStrictEqual(good);

		await store.refresh();
		expect(store.report).toStrictEqual(good);
		expect(store.lastError).toBe('server is busy');
	});

	it('never starts a timer: a history read happens only when asked', async () => {
		vi.useFakeTimers();
		try {
			const fetchUsage = vi.fn(async () => usageReport());
			const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
			store.start();
			await vi.advanceTimersByTimeAsync(0);
			expect(fetchUsage).toHaveBeenCalledTimes(1);

			await vi.advanceTimersByTimeAsync(300_000);
			expect(fetchUsage).toHaveBeenCalledTimes(1);

			await store.refresh();
			expect(fetchUsage).toHaveBeenCalledTimes(2);
		} finally {
			vi.useRealTimers();
		}
	});
});
