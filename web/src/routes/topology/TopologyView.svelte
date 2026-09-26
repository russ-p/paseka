<script lang="ts">
	import Section from '$lib/components/Section.svelte';
	import StatTile from '$lib/components/StatTile.svelte';
	import { copyText } from '$lib/clipboard';
	import { topologyCounts } from '$lib/format';
	import { createTopologyStore, type TopologyStore } from '$lib/stores/topology.svelte';
	import { consoleStatusStore, type ConsoleStatusStore } from '$lib/stores/console-status.svelte';
	import TopologyGraphView from './TopologyGraph.svelte';

	let {
		store = createTopologyStore(),
		status = consoleStatusStore
	}: { store?: TopologyStore; status?: ConsoleStatusStore } = $props();

	$effect(() => {
		store.start();
	});

	let graph = $state<TopologyGraphView | undefined>(undefined);
	let copied = $state(false);

	const topology = $derived(store.topology);
	const counts = $derived(topologyCounts(topology));
	const mermaid = $derived(topology?.mermaid ?? '');

	async function copyMermaid(): Promise<void> {
		if (!(await copyText(mermaid))) return;
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
</script>

<div class="space-y-6">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 space-y-1">
			<h1 class="text-3xl font-bold">Topology</h1>
			<p class="text-base-content/70">
				What can happen in this colony, read from the committed bee YAML and the colony's
				<span class="font-mono">auto_invites</span>. It answers with the hive runtime stopped and
				NATS down, because nothing here is read from the bus.
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<button
				id="topology-copy"
				type="button"
				class="btn btn-sm"
				disabled={!mermaid}
				onclick={() => void copyMermaid()}
			>
				{copied ? 'Copied' : 'Copy Mermaid'}
			</button>
			<button
				id="topology-refresh"
				type="button"
				class="btn btn-sm"
				disabled={store.loading}
				onclick={() => void store.refresh()}
			>
				{store.loading ? 'Refreshing…' : 'Refresh'}
			</button>
			<button
				id="topology-reset"
				type="button"
				class="btn btn-sm"
				disabled={!topology || store.isEmpty}
				onclick={() => graph?.resetLayout()}
			>
				Reset layout
			</button>
		</div>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<div class="grid grid-cols-3 gap-3">
				<span class="skeleton block h-16 w-full"></span>
				<span class="skeleton block h-16 w-full"></span>
				<span class="skeleton block h-16 w-full"></span>
			</div>
			<span class="skeleton block h-96 w-full"></span>
		</div>
	{:else if store.isEmpty}
		<div class="card bg-base-100 shadow-sm">
			<div class="card-body items-center py-16 text-center">
				<h2 class="card-title text-xl">No topology to draw</h2>
				<p class="text-base-content/70">
					This colony has no bees in <span class="font-mono">.paseka/bees/</span> and no
					<span class="font-mono">auto_invites</span>, so nothing is wired to the bus yet.
				</p>
			</div>
		</div>
	{:else if topology}
		<div class="grid grid-cols-3 gap-3">
			{#each counts as count (count.label)}
				<StatTile id={`topology-${count.label.toLowerCase().replace(' ', '-')}`} label={count.label} value={count.value} />
			{/each}
		</div>

		<section id="topology-graph" class="space-y-3">
			<TopologyGraphView bind:this={graph} {topology} colony={status.colony} />
			<p class="text-xs text-base-content/50">
				Bees sit in the left column, event kinds in the right. Solid edges are subscriptions,
				dashed are declared publications, dotted are colony invites, and a faded edge is a
				subscription the bee never declared — an empty
				<span class="font-mono">subscribes</span> means any
				<span class="font-mono">task.ready</span> reaches it. Drag a node to rearrange; the shape
				is remembered per colony. Colours follow the console theme, and the four contracts keep
				one hue each.
			</p>
		</section>

		<!-- The Mermaid is the same graph the CLI prints, and it is the text path to
		     this data: a canvas cannot be read by a screen reader or pasted into a PR. -->
		<Section id="topology-mermaid" title="Mermaid" note="same as paseka colony topology" collapsible>
			{#if mermaid}
				<pre class="max-h-96 overflow-auto rounded-box bg-base-200/50 p-3 font-mono text-xs">{mermaid}</pre>
			{:else}
				<p class="text-base-content/70">
					The server returned no Mermaid for this projection.
				</p>
			{/if}
		</Section>
	{/if}
</div>
