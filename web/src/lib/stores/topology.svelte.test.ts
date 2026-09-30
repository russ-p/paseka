import { describe, expect, it, vi } from 'vitest';
import { createTopologyStore } from './topology.svelte';
import { topology } from '../../tests/fixtures';
import type { Topology } from '$lib/api/types';

function harness(view: Topology = topology()) {
	const loadTopology = vi.fn(async () => view);
	const store = createTopologyStore({ loadTopology });
	return { store, loadTopology };
}

describe('createTopologyStore', () => {
	it('reads once on mount and never polls, because the projection is config-derived', async () => {
		const { store, loadTopology } = harness();

		expect(store.showSkeletons).toBe(true);
		await store.start();
		await store.start();

		// The topology changes when a commit lands, not on a clock, so a second
		// silent read would be a filesystem walk and a YAML parse for nothing.
		expect(loadTopology).toHaveBeenCalledTimes(1);
		expect(store.showSkeletons).toBe(false);
		expect(store.topology?.bees).toHaveLength(3);
	});

	it('keeps the graph on screen through a later failure', async () => {
		const loadTopology = vi
			.fn<() => Promise<Topology>>()
			.mockResolvedValueOnce(topology())
			.mockRejectedValueOnce(new Error('colony.yaml: permission denied'));
		const store = createTopologyStore({ loadTopology });
		await store.start();

		await store.refresh();

		expect(store.lastError).toBe('colony.yaml: permission denied');
		expect(store.topology?.events).toHaveLength(5);
	});

	it('clears the failure once a read succeeds again', async () => {
		const loadTopology = vi
			.fn<() => Promise<Topology>>()
			.mockRejectedValueOnce(new Error('boom'))
			.mockResolvedValueOnce(topology());
		const store = createTopologyStore({ loadTopology });
		await store.start();
		expect(store.lastError).toBe('boom');

		await store.refresh();

		expect(store.lastError).toBe('');
	});

	it('tells an unwired colony apart from a failed read', async () => {
		// "No topology" is the answer for a colony with no bees yet; a failure is an
		// error the operator may be able to fix.
		const { store } = harness(topology({ bees: [], events: [], edges: [], mermaid: '' }));
		await store.start();

		expect(store.isEmpty).toBe(true);
		expect(store.lastError).toBe('');
	});

	it('is not empty when only the bees are missing, since events alone still draw', async () => {
		const { store } = harness(topology({ bees: [] }));
		await store.start();

		expect(store.isEmpty).toBe(false);
	});

	it('is not empty before the first read, so the empty card cannot flash', async () => {
		const { store } = harness();

		expect(store.isEmpty).toBe(false);
	});
});
