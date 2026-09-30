import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { vi as vitest } from 'vitest';
import TopologyView from './TopologyView.svelte';
import { createTopologyStore, type TopologyStore } from '$lib/stores/topology.svelte';
import { createConsoleStatusStore } from '$lib/stores/console-status.svelte';
import { topology } from '../../tests/fixtures';
import type { Topology } from '$lib/api/types';

/**
 * cytoscape needs a laid-out canvas, which jsdom has no notion of, so the graph
 * itself is covered by the unit tests over `lib/topology` and the stylesheet. Here
 * the module is replaced so the page's own contract — counts, actions, states — is
 * what gets exercised.
 */
vitest.mock('cytoscape', () => ({
	default: vi.fn((options: { container: HTMLElement; elements: { data: { id?: string } }[] }) => {
		const nodes = options.elements.filter((element) => element.data.id?.startsWith('bee:'));
		options.container.dataset.testNodes = String(nodes.length);
		options.container.dataset.testCytoscape = 'mounted';
		return {
			destroy: vi.fn(),
			fit: vi.fn(),
			batch: (callback: () => void) => callback(),
			getElementById: () => ({ nonempty: () => true, position: () => ({ x: 0, y: 0 }), id: () => 'x' }),
			nodes: () => ({ forEach: () => {} }),
			on: vi.fn(),
			style: vi.fn()
		};
	})
}));

function harness(view: Topology = topology()): { store: TopologyStore } {
	return { store: createTopologyStore({ loadTopology: async () => view }) };
}

function statusStore(colony = 'apiary') {
	const store = createConsoleStatusStore({ pollIntervalMs: 0 });
	store.applyChromeFrame({
		schemaVersion: 1,
		runtime: { status: 'running', alive: true, slug: 'demo', pid: 42 },
		agents: { count: 0, afk: 0, sessions: 0 },
		attention: { reviews: 0, sessions: 0 }
	});
	// The saved layout is namespaced by colony, and the topbar is where the slug lives.
	return { ...store, colony } as typeof store & { colony: string };
}

describe('topology route', () => {
	it('leads with the counts the legacy console showed', async () => {
		const { store } = harness();
		render(TopologyView, { store, status: statusStore() });
		await waitFor(() => expect(screen.getByText('Bees')).toBeInTheDocument());

		expect(screen.getByText('3')).toBeInTheDocument();
		expect(screen.getByText('Event kinds')).toBeInTheDocument();
		expect(screen.getByText('Rules')).toBeInTheDocument();
	});

	it('draws the graph and says how much of it there is', async () => {
		const { store } = harness();
		const { container } = render(TopologyView, { store, status: statusStore() });
		await waitFor(() =>
			expect(container.querySelector('[data-test-cytoscape="mounted"]')).not.toBeNull()
		);

		const graph = screen.getByRole('img');
		// A canvas is opaque to a screen reader, so the label carries the size and
		// points at the Mermaid, which is the same data as text.
		expect(graph).toHaveAccessibleName(/3 bees and 5 event kinds/);
		expect(graph).toHaveAccessibleName(/Mermaid below/);
	});

	it('keeps the three actions, because a graph needs them and the audit only wanted two hidden', async () => {
		const { store } = harness();
		render(TopologyView, { store, status: statusStore() });
		await waitFor(() => expect(screen.getByText('Bees')).toBeInTheDocument());

		expect(screen.getByRole('button', { name: 'Copy Mermaid' })).toBeEnabled();
		expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled();
		expect(screen.getByRole('button', { name: 'Reset layout' })).toBeEnabled();
	});

	it('offers the Mermaid as the text form of the same graph', async () => {
		const { store } = harness();
		render(TopologyView, { store, status: statusStore() });
		await waitFor(() => expect(screen.getByText('Bees')).toBeInTheDocument());

		await userEvent.click(screen.getByText('Mermaid'));

		expect(screen.getByText(/bee:scout --> event:task.ready/)).toBeInTheDocument();
	});

	it('says when the server returned no Mermaid rather than an empty box', async () => {
		const { store } = harness(topology({ mermaid: '' }));
		render(TopologyView, { store, status: statusStore() });
		await waitFor(() => expect(screen.getByText('Bees')).toBeInTheDocument());

		expect(screen.getByRole('button', { name: 'Copy Mermaid' })).toBeDisabled();
	});

	it('refreshes on demand, because the projection only changes on a commit', async () => {
		const loadTopology = vi.fn(async () => topology());
		const store = createTopologyStore({ loadTopology });
		render(TopologyView, { store, status: statusStore() });
		await waitFor(() => expect(loadTopology).toHaveBeenCalledTimes(1));

		await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));

		await waitFor(() => expect(loadTopology).toHaveBeenCalledTimes(2));
	});

	it('says an unwired colony is empty rather than broken', async () => {
		const { store } = harness(topology({ bees: [], events: [], edges: [], mermaid: '' }));
		render(TopologyView, { store, status: statusStore() });
		await waitFor(() => expect(screen.getByText('No topology to draw')).toBeInTheDocument());

		expect(screen.getByText(/no bees in/)).toBeInTheDocument();
		// Nothing to lay out, so the control would have nothing to do.
		expect(screen.getByRole('button', { name: 'Reset layout' })).toBeDisabled();
		expect(screen.queryByRole('img')).not.toBeInTheDocument();
	});

	it('alerts and drops the skeletons when the read fails outright', async () => {
		const store = createTopologyStore({
			loadTopology: async () => {
				throw new Error('colony.yaml: no such file');
			}
		});
		render(TopologyView, { store, status: statusStore() });
		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('colony.yaml: no such file'));

		expect(document.querySelector('[aria-busy="true"]')).toBeNull();
	});

	it('shows skeletons before the first projection', () => {
		const { store } = harness();
		render(TopologyView, { store, status: statusStore() });

		expect(document.querySelector('[aria-busy="true"]')).not.toBeNull();
		expect(screen.queryByText('Bees')).not.toBeInTheDocument();
	});
});
