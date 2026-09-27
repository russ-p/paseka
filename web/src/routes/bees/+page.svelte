<script lang="ts">
	import { base } from '$app/paths';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import {
		beeIntentsLabel,
		beeIsInteractive,
		beeLastRunLabel,
		beeLiveCount,
		beesHeadline,
		beeWorkspaceLabel
	} from '$lib/format';
	import { runDetailPath } from '$lib/navigation';
	import { createBeesStore, type BeesStore } from '$lib/stores/bees.svelte';
	import { consoleStatusStore, type ConsoleStatusStore } from '$lib/stores/console-status.svelte';
	import type { Bee } from '$lib/api/types';

	let {
		store = createBeesStore(),
		status = consoleStatusStore
	}: { store?: BeesStore; status?: ConsoleStatusStore } = $props();

	$effect(() => {
		store.start();
	});

	const bees = $derived(store.bees ?? []);

	/**
	 * Whether a bee is live is joined from the agents frame the topbar already
	 * keeps, so the column costs no request and cannot disagree with the Live bees
	 * plaque above it — the same read `/next/system` makes.
	 */
	function liveCount(bee: Bee): number {
		return beeLiveCount(status.agents?.items, bee.role);
	}

	const beeColumns: DataColumn<Bee>[] = [
		{
			key: 'role',
			label: 'Bee',
			text: (bee) => bee.role,
			searchText: (bee) => (beeIsInteractive(bee) ? '' : 'script non-interactive'),
			mono: true
		},
		{
			key: 'adapter',
			label: 'Adapter',
			text: (bee) => bee.adapter,
			searchText: (bee) => (beeIsInteractive(bee) ? '' : 'script non-interactive')
		},
		{
			key: 'intents',
			label: 'Intents',
			// One comma-joined string, not a badge per intent: `text` always feeds the
			// filter box, so this makes each intent individually searchable for free,
			// and `whitespace-nowrap` forbids the row height a badge list would want.
			text: (bee) => beeIntentsLabel(bee),
			secondary: true,
			grow: true
		},
		{
			key: 'sector',
			label: 'Sector',
			text: (bee) => bee.sector || '—',
			mono: true,
			secondary: true
		},
		{
			key: 'workspace',
			label: 'Workspace',
			// Plain text, not a badge: this is committed config rather than a state
			// that moves, so a word says it and the colour would only be decoration.
			// A `badge: false` column would fall back to nothing on a colony-root bee,
			// which is exactly the row that needs saying out loud.
			text: (bee) => beeWorkspaceLabel(bee)
		},
		{
			key: 'live',
			label: 'Live',
			text: (bee) => String(liveCount(bee)),
			// A null badge is the empty cell, which here means the bee holds no live
			// adapter process — the normal state for most of a colony, so a word in
			// every idle row would be noise rather than news.
			badge: (bee) => {
				const count = liveCount(bee);
				return count > 0 ? { status: 'live', label: String(count) } : null;
			}
		},
		{
			key: 'lastRun',
			label: 'Last run',
			text: (bee) => beeLastRunLabel(bee),
			// The run is a link because the run page is where a bee's output is read;
			// a bee that never ran has nowhere to go and stays plain text.
			href: (bee) =>
				bee.lastRun ? runDetailPath(base, bee.lastRun.traceId, bee.lastRun.agentId) : null,
			// Declared so the state rides beside the time, and so a bee that has never
			// run leaves the cell empty rather than printing a bare timestamp.
			badge: (bee) => (bee.lastRun ? { status: bee.lastRun.state, label: bee.lastRun.state } : null)
		}
	];
</script>

<svelte:head>
	<title>Bees · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 max-w-3xl space-y-1">
			<h1 class="text-3xl font-bold">Bees</h1>
			<p class="text-base-content/70">{beesHeadline(bees.length)}</p>
			<!-- The roster is every bee, not only the launchable ones, so the two things an
			     operator cannot see anywhere else are said here: a script bee runs headless
			     and cannot be started as a session, and a colony-root bee writes to the
			     checkout itself rather than to an isolated worktree. -->
			<p class="text-xs text-base-content/50">
				Every bee in <span class="font-mono">.paseka/bees</span>, including the ones an adapter
				runs headless — a <span class="font-mono">script</span> bee cannot be started as a
				interactive session, so it is listed without the intents a launch would offer. A bee
				marked <span class="font-mono">worktree</span> mutates code inside
				<span class="font-mono">.paseka/worktrees</span>; the rest work against the colony root.
			</p>
		</div>
		<button
			id="bees-refresh"
			type="button"
			class="btn btn-sm"
			disabled={store.loading}
			onclick={() => void store.refresh()}
		>
			Refresh
		</button>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-64 w-full"></span>
		</div>
	{:else if store.bees}
		<DataTable
			label="Bees"
			columns={beeColumns}
			rows={bees}
			rowKey={(bee) => bee.role}
			emptyMessage="No bees are registered under .paseka/bees."
			filterLabel="Filter bees"
			filterPlaceholder="role, adapter, intent, sector"
			pageSize={10}
		/>
	{/if}
</div>
