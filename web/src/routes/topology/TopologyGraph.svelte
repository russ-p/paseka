<script lang="ts">
	import type { Core } from 'cytoscape';
	import type { Topology } from '$lib/api/types';
	import {
		bipartiteLayout,
		clearTopologyPositions,
		loadTopologyPositions,
		saveTopologyPositions,
		topologyElements,
		type SavedPositions
	} from '$lib/topology';
	import { readTopologyThemeTokens, topologyStylesheet } from '$lib/topology-theme';
	import { themeStore } from '$lib/stores/theme.svelte';

	let {
		topology,
		colony,
		label = 'Colony topology diagram'
	}: { topology: Topology; colony?: string; label?: string } = $props();

	let container = $state<HTMLDivElement | null>(null);
	let instance = $state<Core | undefined>(undefined);

	function storage(): Storage | undefined {
		return typeof localStorage === 'undefined' ? undefined : localStorage;
	}

	/**
	 * Place every node, then let any dragged shape win. Saved positions are applied
	 * after the computed layout so an operator who pulled the graph into something
	 * they can read keeps it across a re-read and a reload; whatever is left over
	 * falls back to the crossing-reduced blocks.
	 */
	function applyLayout(cy: Core, view: Topology, saved: SavedPositions | null): void {
		const { nodes, edges } = topologyElements(view);
		const beeIds = nodes.filter((node) => node.nodeType === 'bee').map((node) => node.id);
		const eventIds = nodes.filter((node) => node.nodeType === 'event').map((node) => node.id);
		const layout = bipartiteLayout(beeIds, eventIds, edges);
		cy.batch(() => {
			for (const node of [...layout.bees, ...layout.events]) {
				cy.getElementById(node.id).position({ x: node.x, y: node.y });
			}
			if (saved) {
				for (const [id, point] of Object.entries(saved)) {
					const node = cy.getElementById(id);
					if (node.nonempty() && Number.isFinite(point?.x) && Number.isFinite(point?.y)) {
						node.position({ x: point.x, y: point.y });
					}
				}
			}
		});
		cy.fit(undefined, 40);
	}

	function persist(cy: Core): void {
		const positions: SavedPositions = {};
		cy.nodes().forEach((node) => {
			const point = node.position();
			positions[node.id()] = { x: point.x, y: point.y };
		});
		saveTopologyPositions(storage(), colony, positions);
	}

	/** Back to the computed rows, and forget the saved shape so it cannot come back. */
	export function resetLayout(): void {
		if (!instance || !topology) return;
		clearTopologyPositions(storage(), colony);
		applyLayout(instance, topology, null);
	}

	/**
	 * cytoscape is the largest dependency in the console and only this route draws a
	 * graph, so it is imported on demand: it lands in this route's chunk and not in
	 * the initial bundle every other route also pays for.
	 *
	 * One effect owns the instance. A new projection is a new graph — node ids are
	 * `bee:<role>` and `event:<id>`, so an id can survive a re-read while meaning
	 * something else — so the old instance is destroyed rather than patched.
	 */
	$effect(() => {
		// Read before the await, so this effect depends on the projection and rebuilds
		// when it changes.
		const view = topology;
		const node = container;
		if (!view || !node || view.bees.length === 0 && view.events.length === 0) return;
		let disposed = false;
		let cy: Core | undefined;

		void (async () => {
			const { default: cytoscape } = await import('cytoscape');
			if (disposed) return;
			const { nodes, edges } = topologyElements(view);
			cy = cytoscape({
				container: node,
				elements: [...nodes, ...edges].map((element) => ({ data: element })),
				style: topologyStylesheet(readTopologyThemeTokens()),
				layout: { name: 'preset' },
				minZoom: 0.2,
				maxZoom: 3,
				wheelSensitivity: 0.3
			});
			instance = cy;
			applyLayout(cy, view, loadTopologyPositions(storage(), colony));
			cy.on('dragfree', 'node', () => persist(cy as Core));
		})();

		return () => {
			disposed = true;
			cy?.destroy();
			instance = undefined;
		};
	});

	// A theme switch changes the tokens the stylesheet was built from, so they are
	// re-read and the live instance restyled. Rebuilding instead would be simpler and
	// wrong: it would throw away the operator's in-progress drag.
	$effect(() => {
		void themeStore.current;
		const tokens = readTopologyThemeTokens();
		instance?.style(topologyStylesheet(tokens));
	});
</script>

<div
	bind:this={container}
	class="h-[32rem] w-full rounded-box border border-base-300 bg-base-200/30"
	role="img"
	aria-label="{label}: {topology.bees.length} bees and {topology.events.length} event kinds. The same graph is available as Mermaid below."
></div>
