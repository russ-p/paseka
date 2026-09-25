<script lang="ts">
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import {
		consoleStatusStore,
		type ConsoleStatusStore
	} from '$lib/stores/console-status.svelte';

	let { store = consoleStatusStore }: { store?: ConsoleStatusStore } = $props();

	const natsStatus = $derived(store.natsStatus);
	const runtimeStatus = $derived(store.runtimeStatus);
</script>

<header class="sticky top-0 z-30 border-b border-base-300 bg-base-100" aria-label="Colony status">
	<div class="navbar min-h-16 flex-wrap gap-3 overflow-x-auto px-3 md:flex-nowrap md:px-6">
		<div class="w-full shrink-0 pl-16 md:min-w-52 md:w-auto md:flex-1">
			<p class="truncate text-lg font-semibold">Queen Console</p>
			<p class="truncate text-xs text-base-content/60">{store.colony ?? 'Colony unknown'}</p>
		</div>

		<div class="flex min-w-0 flex-wrap items-center gap-2 md:flex-nowrap">
			{#if store.loading}
				<span class="skeleton h-6 w-20"></span>
				<span class="skeleton h-6 w-24"></span>
				<span class="skeleton h-6 w-16"></span>
			{:else}
				<StatusBadge
					status={natsStatus}
					label={natsStatus === 'connected'
						? 'NATS'
						: natsStatus === 'idle'
							? 'NATS off'
							: 'NATS unknown'}
				/>
				<StatusBadge
					status={runtimeStatus}
					label={runtimeStatus === 'unknown' ? 'Hive unknown' : `Hive ${runtimeStatus}`}
				/>
				<span class="badge badge-ghost whitespace-nowrap">Bees {store.liveBees}</span>
				<span class="badge badge-outline whitespace-nowrap">Recent trails {store.activeTraceCount}</span>
				{#if store.reviews > 0}
					<StatusBadge status="pending" label={`Reviews ${store.reviews}`} />
				{/if}
				{#if store.pendingSessions > 0}
					<StatusBadge status="pending" label={`Invites ${store.pendingSessions}`} />
				{/if}
			{/if}
		</div>

		<a class="btn btn-ghost btn-sm hidden sm:inline-flex" href="/">Legacy</a>
	</div>

	{#if store.connection !== 'connected'}
		<div class="flex items-center gap-2 border-t border-warning/20 bg-warning/10 px-4 py-2" role="status">
			<StatusBadge status={store.connection} />
			<span class="text-sm">
				{store.connection === 'reconnecting'
					? 'Reconnecting to the console event stream.'
					: 'The console event stream is unavailable.'}
			</span>
		</div>
	{/if}

	{#if store.lastError}
		<div class="alert alert-error mx-3 my-2 md:mx-6" role="alert">
			<span>{store.lastError}</span>
		</div>
	{/if}
</header>
