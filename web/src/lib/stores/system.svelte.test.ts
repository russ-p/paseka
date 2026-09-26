import { describe, expect, it, vi } from 'vitest';
import { createSystemStore } from './system.svelte';
import { systemView } from '../../tests/fixtures';
import type { SystemView } from '$lib/api/types';

/** A store that never polls unless told to. */
function harness(overrides: Partial<Parameters<typeof createSystemStore>[0]> = {}, view?: SystemView) {
	const loadSystem = vi.fn(async () => view ?? systemView());
	const options = { loadSystem, pollIntervalMs: 0, ...overrides };
	return { store: createSystemStore(options), ...options };
}

describe('createSystemStore', () => {
	it('skeletons until the first payload', async () => {
		const { store } = harness();

		expect(store.showSkeletons).toBe(true);
		await store.refresh();

		expect(store.showSkeletons).toBe(false);
		expect(store.system).not.toBeNull();
	});

	it('keeps the last real numbers when a read fails, because stale beats blank', async () => {
		const loadSystem = vi
			.fn<() => Promise<SystemView>>()
			.mockResolvedValueOnce(systemView())
			.mockRejectedValueOnce(new Error('connection refused'));
		const store = createSystemStore({ loadSystem, pollIntervalMs: 0 });
		await store.refresh();

		await store.refresh();

		expect(store.lastError).toBe('connection refused');
		expect(store.system?.hostname).toBe('apiary');
		expect(store.processes).toHaveLength(3);
	});

	it('clears the failure once a read succeeds again', async () => {
		const loadSystem = vi
			.fn<() => Promise<SystemView>>()
			.mockRejectedValueOnce(new Error('connection refused'))
			.mockResolvedValueOnce(systemView());
		const store = createSystemStore({ loadSystem, pollIntervalMs: 0 });
		await store.refresh();
		expect(store.lastError).toBe('connection refused');

		await store.refresh();

		expect(store.lastError).toBe('');
	});

	it('keeps the server own partial note apart from a failed read', async () => {
		// The server answers 200 with an `error` string when part of the snapshot
		// failed, so it must not read as the page having broken.
		const { store } = harness({}, systemView({ error: 'loadavg: permission denied' }));
		await store.refresh();

		expect(store.partialError).toBe('loadavg: permission denied');
		expect(store.lastError).toBe('');
	});

	it('reports an absent process list as empty rather than undefined', async () => {
		// Off Linux the server omits the list entirely, which is not an empty host.
		const { store } = harness({}, systemView({ processes: undefined }));
		await store.refresh();

		expect(store.processes).toEqual([]);
		expect(store.system?.cpus).toBe(8);
	});

	it('polls on an interval and stops on unmount, so an idle console reads no /proc', async () => {
		vi.useFakeTimers();
		try {
			const loadSystem = vi.fn(async () => systemView());
			const store = createSystemStore({ loadSystem, pollIntervalMs: 1000 });

			store.start();
			store.start();
			await vi.advanceTimersByTimeAsync(1000);
			expect(loadSystem).toHaveBeenCalledTimes(2);

			store.stop();
			await vi.advanceTimersByTimeAsync(5000);
			expect(loadSystem).toHaveBeenCalledTimes(2);
		} finally {
			vi.useRealTimers();
		}
	});
});
