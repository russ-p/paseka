import { describe, expect, it, vi } from 'vitest';
import { createBeesStore } from './bees.svelte';
import { beeRoster } from '../../tests/fixtures';
import type { Bee } from '$lib/api/types';

/** The store's own reader, so a wrong scope on the default path is assertable. */
const listBees = vi.fn(async (_scope?: string): Promise<Bee[]> => beeRoster());
vi.mock('$lib/api/client', () => ({ listBees: (scope?: string) => listBees(scope) }));

function harness(bees: Bee[] = beeRoster()) {
	const loadBees = vi.fn(async () => bees);
	const store = createBeesStore({ loadBees });
	return { store, loadBees };
}

describe('createBeesStore', () => {
	it('reads once on mount and never polls, because a bee roster is committed config', async () => {
		const { store, loadBees } = harness();

		expect(store.showSkeletons).toBe(true);
		await store.start();
		await store.start();

		// Identity, adapter, sector, and intents come from bee YAML, so they change
		// when a commit lands. A timer would re-read them for nothing; the liveness
		// that does move arrives on the chrome stream instead.
		expect(loadBees).toHaveBeenCalledTimes(1);
		expect(store.showSkeletons).toBe(false);
		expect(store.bees).toHaveLength(3);
	});

	it('asks for the colony scope, so a script bee the picker hides is on the roster', async () => {
		await createBeesStore().start();

		// Without the scope the store would be handed the launch picker, and a colony
		// whose bees are all `script` would read as a colony with no bees at all.
		expect(listBees).toHaveBeenCalledWith('colony');
	});

	it('keeps the roster on screen through a later failure', async () => {
		const loadBees = vi
			.fn<() => Promise<Bee[]>>()
			.mockResolvedValueOnce(beeRoster())
			.mockRejectedValueOnce(new Error('bees: permission denied'));
		const store = createBeesStore({ loadBees });
		await store.start();

		await store.refresh();

		expect(store.lastError).toBe('bees: permission denied');
		expect(store.bees).toHaveLength(3);
		expect(store.showSkeletons).toBe(false);
	});

	it('clears the failure once a read succeeds again', async () => {
		const loadBees = vi
			.fn<() => Promise<Bee[]>>()
			.mockRejectedValueOnce(new Error('boom'))
			.mockResolvedValueOnce(beeRoster());
		const store = createBeesStore({ loadBees });
		await store.start();
		expect(store.lastError).toBe('boom');

		await store.refresh();

		expect(store.lastError).toBe('');
		expect(store.bees).toHaveLength(3);
	});

	it('reports a colony with no bees as empty rather than as a failure to read it', async () => {
		const { store } = harness([]);
		await store.start();

		expect(store.isEmpty).toBe(true);
		expect(store.lastError).toBe('');
		// Before the first read, emptiness is unknown rather than false.
		expect(createBeesStore().isEmpty).toBe(false);
	});

	it('re-reads on demand, because a run landing is the one thing that moves without a commit', async () => {
		const { store, loadBees } = harness();
		await store.start();

		await store.refresh();

		expect(loadBees).toHaveBeenCalledTimes(2);
	});
});
