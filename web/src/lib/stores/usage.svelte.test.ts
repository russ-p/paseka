import { describe, expect, it, vi } from 'vitest';
import {
	createUsageStore,
	inclusiveEnd,
	usageCustomRange,
	usagePresetRange
} from './usage.svelte';
import { usageReport } from '../../tests/fixtures';
import type { UsageReport } from '$lib/api/types';

type UsageQuery = {
	since: string;
	until: string;
	timezone: string;
	refresh?: boolean;
	adapter: string[];
	model: string[];
	bee: string[];
	purpose: string[];
};

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
		expect(store.lastErrorKind).toBe('read-failed');
	});

	it('refuses a range over the cap before walking run directories', async () => {
		const good = usageReport();
		const fetchUsage = vi.fn(async (_query: UsageQuery) => good);
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		store.start();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		store.applyCustom('2020-01-01', '2026-09-30');
		// The cap is a bound the page refuses itself: no read is sent for it, and
		// the last good report is still the report.
		expect(fetchUsage).toHaveBeenCalledTimes(1);
		expect(store.lastErrorKind).toBe('range-too-long');
		expect(store.lastError).toContain('366');
		expect(store.report).toStrictEqual(good);
	});

	it('reads a server refusal of an over-long range as the same state', async () => {
		const fetchUsage = vi
			.fn<() => Promise<UsageReport>>()
			.mockResolvedValueOnce(usageReport())
			.mockRejectedValueOnce(new Error('range is longer than 366 days'));
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		await store.refresh();
		await store.refresh();

		expect(store.lastErrorKind).toBe('range-too-long');
		expect(store.report).not.toBeNull();
	});

	it('bypasses the scan cache only for the explicit refresh', async () => {
		const fetchUsage = vi.fn(async (_query: UsageQuery) => usageReport());
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		store.start();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));
		// Mounting and a range change may reuse the server's parsed scan.
		expect(fetchUsage.mock.calls[0][0]).not.toHaveProperty('refresh');

		store.applyPreset(7);
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(fetchUsage.mock.calls[1][0]).not.toHaveProperty('refresh');

		// The refresh control is the one read that asks for a new scan.
		await store.refresh();
		expect(fetchUsage).toHaveBeenCalledTimes(3);
		expect(fetchUsage.mock.calls[2][0]).toMatchObject({ refresh: true });
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

describe('createUsageStore filters and address bar', () => {	it('seeds the view from the address bar and sends the filters once', async () => {
		window.history.replaceState(
			null,
			'',
			'/usage?since=2026-09-01&until=2026-09-15&axis=model&adapter=opencode&bee=builder&bee=guard'
		);
		const fetchUsage = vi.fn(async (_query: UsageQuery) => usageReport());
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });

		store.start();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		expect(fetchUsage).toHaveBeenCalledWith({
			since: '2026-09-01',
			until: '2026-09-15',
			timezone: 'UTC',
			adapter: ['opencode'],
			model: [],
			bee: ['builder', 'guard'],
			purpose: []
		});
		expect(store.axis).toBe('model');
		expect(store.preset).toBe('custom');
		expect(store.filters.bee).toEqual(['builder', 'guard']);
	});

	it('issues one read per filter change and none for an axis change', async () => {
		const fetchUsage = vi.fn(async (_query: UsageQuery) => usageReport());
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		store.start();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		store.setAxis('model');
		await Promise.resolve();
		expect(fetchUsage).toHaveBeenCalledTimes(1);

		store.addFilter('model', 'openai/gpt-5');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(store.filters.model).toEqual(['openai/gpt-5']);

		// A value already present, and one that is not there to remove, are no
		// changes: neither may cost a read.
		store.addFilter('model', 'openai/gpt-5');
		store.removeFilter('bee', 'builder');
		await Promise.resolve();
		expect(fetchUsage).toHaveBeenCalledTimes(2);

		store.removeFilter('model', 'openai/gpt-5');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(3));
		store.addFilter('adapter', 'opencode');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(4));
		store.clearFilters();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(5));
		expect(store.hasFilters).toBe(false);
	});

	it('publishes the range, axis, and filters with replace semantics', async () => {
		const fetchUsage = vi.fn(async (_query: UsageQuery) => usageReport());
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		const before = window.history.length;

		store.start();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));
		const mounted = new URLSearchParams(window.location.search);
		expect(mounted.get('since')).toBe('2026-09-01');
		expect(mounted.get('until')).toBe('2026-10-01');
		expect(mounted.get('axis')).toBeNull();

		store.setAxis('bee');
		expect(new URLSearchParams(window.location.search).get('axis')).toBe('bee');

		store.addFilter('bee', 'builder');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		const current = new URLSearchParams(window.location.search);
		expect(current.get('axis')).toBe('bee');
		expect(current.getAll('bee')).toEqual(['builder']);

		store.clearFilters();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(3));
		expect(new URLSearchParams(window.location.search).getAll('bee')).toEqual([]);

		// Replace, not push: the address bar never becomes a stack of values.
		expect(window.history.length).toBe(before);
	});
});

describe('createUsageStore drill-down', () => {
	it('narrows to a selected day, keeps the filters and the range, and restores on clear', async () => {
		window.history.replaceState(null, '', '/usage');
		const fetchUsage = vi.fn(async (_query: UsageQuery) => usageReport());
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		store.start();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		store.addFilter('model', 'openai/gpt-5');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));

		store.selectDay('2026-09-25');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(3));
		expect(fetchUsage).toHaveBeenLastCalledWith({
			since: '2026-09-25',
			until: '2026-09-26',
			timezone: 'UTC',
			adapter: [],
			model: ['openai/gpt-5'],
			bee: [],
			purpose: []
		});
		expect(store.day).toBe('2026-09-25');
		// The chosen range is still the context; only the day narrows the read.
		expect(store.since).toBe('2026-09-01');
		expect(store.until).toBe('2026-10-01');
		expect(new URLSearchParams(window.location.search).get('day')).toBe('2026-09-25');

		// Asking for the day already in view undoes the narrowing, so one
		// control can reverse it.
		store.selectDay('2026-09-25');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(4));
		expect(store.day).toBeNull();
		expect(fetchUsage).toHaveBeenLastCalledWith(
			expect.objectContaining({
				since: '2026-09-01',
				until: '2026-10-01',
				model: ['openai/gpt-5']
			})
		);
		expect(new URLSearchParams(window.location.search).has('day')).toBe(false);

		store.selectDay('2026-09-26');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(5));
		store.clearDay();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(6));
		expect(store.day).toBeNull();
	});

	it('seeds a drilled day from the address bar and reads just that day', async () => {
		window.history.replaceState(
			null,
			'',
			'/usage?since=2026-09-01&until=2026-10-01&day=2026-09-25&model=openai%2Fgpt-5'
		);
		const fetchUsage = vi.fn(async (_query: UsageQuery) => usageReport());
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		store.start();

		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));
		expect(fetchUsage).toHaveBeenCalledWith({
			since: '2026-09-25',
			until: '2026-09-26',
			timezone: 'UTC',
			adapter: [],
			model: ['openai/gpt-5'],
			bee: [],
			purpose: []
		});
		expect(store.day).toBe('2026-09-25');
	});

	it('leaves the day when a new range is chosen', async () => {
		window.history.replaceState(null, '', '/usage?day=2026-09-25');
		const fetchUsage = vi.fn(async (_query: UsageQuery) => usageReport());
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		store.start();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		store.applyPreset(7);
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(store.day).toBeNull();
		expect(fetchUsage).toHaveBeenLastCalledWith(
			expect.objectContaining({ since: '2026-09-24', until: '2026-10-01' })
		);
		expect(new URLSearchParams(window.location.search).has('day')).toBe(false);
	});

	it('toggles a breakdown key into and out of the filters', async () => {
		window.history.replaceState(null, '', '/usage');
		const fetchUsage = vi.fn(async (_query: UsageQuery) => usageReport());
		const store = createUsageStore({ fetchUsage, now: fixedNow, timezone: 'UTC' });
		store.start();
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(1));

		store.toggleFilter('model', 'openai/gpt-5');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(2));
		expect(store.filters.model).toEqual(['openai/gpt-5']);

		store.toggleFilter('model', 'openai/gpt-5');
		await vi.waitFor(() => expect(fetchUsage).toHaveBeenCalledTimes(3));
		expect(store.filters.model).toEqual([]);
	});
});
