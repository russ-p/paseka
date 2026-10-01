<script lang="ts">
	import { Check, Copy, RefreshCw, RotateCcw } from 'lucide-svelte';
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
	<!-- Reading the projection again and throwing the operator's dragged shape away are
	     page-level acts, so they take the header's action slot on the right, as on the Git
	     route; copying the Mermaid acts on the block, so it rides that block's header. The
	     `flex-1` on the title column is what keeps them on the title's row: a flex item's
	     base size is its max-content, so a description longer than the free space wraps them
	     onto a line of their own, where `justify-between` has nothing to push apart. -->
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 flex-1 space-y-1">
			<h1 class="text-3xl font-bold">Topology</h1>
			<p class="text-base-content/70">
				What can happen in this colony, read from the committed bee YAML and the colony's
				<span class="font-mono">auto_invites</span>. It answers with the hive runtime stopped and
				NATS down, because nothing here is read from the bus.
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<button
				id="topology-refresh"
				type="button"
				class="btn btn-sm"
				disabled={store.loading}
				aria-busy={store.loading}
				onclick={() => void store.refresh()}
			>
				<RefreshCw class="h-4 w-4 {store.loading ? 'animate-spin' : ''}" strokeWidth={2.5} />
				Refresh
			</button>
			<button
				id="topology-reset"
				type="button"
				class="btn btn-error btn-sm btn-outline"
				disabled={!topology || store.isEmpty}
				onclick={() => graph?.resetLayout()}
			>
				<RotateCcw class="h-4 w-4" strokeWidth={2.5} />
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
		     this data: a canvas cannot be read by a screen reader or pasted into a PR. It
		     starts folded, since the graph above is the reason to be here and this is its
		     text shadow; copy rides the block header rather than the page header, so the
		     control that acts on it is the one attached to it. -->
		<Section
			id="topology-mermaid"
			title="Mermaid"
			note="same as paseka colony topology"
			collapsible
			open={false}
		>
			{#snippet actions()}
				<button
					id="topology-copy"
					type="button"
					class="btn btn-ghost btn-xs"
					disabled={!mermaid}
					aria-label={copied ? 'Copied' : 'Copy'}
					title={copied ? 'Copied' : 'Copy'}
					onclick={() => void copyMermaid()}
				>
					{#if copied}
						<Check class="h-3.5 w-3.5 text-success" strokeWidth={2.5} />
					{:else}
						<Copy class="h-3.5 w-3.5" strokeWidth={2} />
					{/if}
				</button>
			{/snippet}
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
