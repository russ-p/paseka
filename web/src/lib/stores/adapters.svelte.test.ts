import { describe, expect, it, vi } from 'vitest';
import { createAdapterCLIsStore } from './adapters.svelte';
import { adapterCLIs } from '../../tests/fixtures';
import type { AdapterCLIs } from '$lib/api/types';

function harness(
	overrides: Partial<Parameters<typeof createAdapterCLIsStore>[0]> = {},
	view?: AdapterCLIs
) {
	const loadAdapters = vi.fn(async () => view ?? adapterCLIs());
	const options = { loadAdapters, ...overrides };
	return { store: createAdapterCLIsStore(options), ...options };
}

describe('createAdapterCLIsStore', () => {
	it('reads nothing until the block is opened', async () => {
		// Arriving on System must not exec four external binaries: the block is
		// folded, the server caches the answer, and nobody has asked for it yet.
		const { store, loadAdapters } = harness();

		expect(loadAdapters).not.toHaveBeenCalled();
		expect(store.probed).toBe(false);
		expect(store.adapters).toEqual([]);

		await store.load();

		expect(loadAdapters).toHaveBeenCalledTimes(1);
		expect(loadAdapters).toHaveBeenCalledWith(false);
		expect(store.probed).toBe(true);
		expect(store.adapters).toHaveLength(4);
	});

	it('asks once for two opens, because the server caches the answer', async () => {
		const { store, loadAdapters } = harness();

		await store.load();
		await store.load();

		expect(loadAdapters).toHaveBeenCalledTimes(1);
	});

	it('re-reads on refresh and asks the server to drop its cache', async () => {
		const second = adapterCLIs({
			adapters: [{ ...adapterCLIs().adapters[0], version: '0.49.0' }]
		});
		const loadAdapters = vi
			.fn<() => Promise<AdapterCLIs>>()
			.mockResolvedValueOnce(adapterCLIs())
			.mockResolvedValueOnce(second);
		const store = createAdapterCLIsStore({ loadAdapters });
		await store.load();

		await store.refresh();

		expect(loadAdapters).toHaveBeenLastCalledWith(true);
		expect(store.adapters[0].version).toBe('0.49.0');
	});

	it('keeps the rows and says why when a read fails, and retries on the next open', async () => {
		// The block folds and reopens, so a failed probe has to be recoverable
		// rather than leaving an empty table and a dead Refresh.
		const loadAdapters = vi
			.fn<() => Promise<AdapterCLIs>>()
			.mockRejectedValueOnce(new Error('connection refused'))
			.mockResolvedValueOnce(adapterCLIs());
		const store = createAdapterCLIsStore({ loadAdapters });

		await store.load();
		expect(store.lastError).toBe('connection refused');
		expect(store.probed).toBe(false);

		await store.load();

		expect(store.lastError).toBe('');
		expect(store.adapters).toHaveLength(4);
	});

	it('refuses a second read while one is in flight', async () => {
		let release: (view: AdapterCLIs) => void = () => {};
		const loadAdapters = vi.fn(
			() =>
				new Promise<AdapterCLIs>((resolve) => {
					release = resolve;
				})
		);
		const store = createAdapterCLIsStore({ loadAdapters });

		const first = store.load();
		await store.load();
		expect(loadAdapters).toHaveBeenCalledTimes(1);

		release(adapterCLIs());
		await first;
		expect(store.adapters).toHaveLength(4);
	});
});
