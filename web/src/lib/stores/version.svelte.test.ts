import { describe, expect, it, vi } from 'vitest';
import { createVersionStore } from './version.svelte';
import { buildView } from '../../tests/fixtures';
import type { BuildView } from '$lib/api/types';

describe('createVersionStore', () => {
	it('reads once and never starts a timer, because a build cannot change under a page', () => {
		const loadVersion = vi.fn(async () => buildView());
		const store = createVersionStore({ loadVersion });

		store.start();
		store.start();

		// A poller here would re-read a constant forever; the whole point of this
		// store is that there is nothing to watch.
		expect(loadVersion).toHaveBeenCalledTimes(1);
		expect(store.build).toBeNull();
	});

	it('carries the whole stamp the server composed', async () => {
		const store = createVersionStore({ loadVersion: async () => buildView() });
		await store.refresh();

		// The side menu formats from this rather than assembling its own answer, so
		// the stamp it shows is the stamp the binary reported.
		expect(store.build?.display).toBe('0.5.0+67730c4');
		expect(store.build?.commitUrl).toBe(
			'https://github.com/russ-p/paseka/commit/67730c4da70fbd912a575fe614e5e248c402fdf0'
		);
	});

	it('says nothing rather than claiming a version it does not have', async () => {
		const loadVersion = vi.fn<() => Promise<BuildView>>().mockRejectedValue(new Error('connection refused'));
		const store = createVersionStore({ loadVersion });
		await store.refresh();

		// The footer is where an operator confirms which build they are reading, so a
		// dash there would read as an answer. An absent stamp is the honest state.
		expect(store.build).toBeNull();
		expect(store.lastError).toBe('connection refused');
	});

	it('recovers after a failed read', async () => {
		const loadVersion = vi
			.fn<() => Promise<BuildView>>()
			.mockRejectedValueOnce(new Error('connection refused'))
			.mockResolvedValueOnce(buildView());
		const store = createVersionStore({ loadVersion });
		await store.refresh();
		expect(store.build).toBeNull();

		await store.refresh();

		expect(store.build?.display).toBe('0.5.0+67730c4');
		expect(store.lastError).toBe('');
	});
});
