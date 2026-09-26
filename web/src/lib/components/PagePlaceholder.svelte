<script lang="ts">
	import StatusBadge from '$lib/components/StatusBadge.svelte';

	let {
		title,
		description,
		emptyMessage,
		level = 1,
		setPageTitle = true
	}: {
		title: string;
		description: string;
		emptyMessage: string;
		level?: 1 | 2;
		setPageTitle?: boolean;
	} = $props();
</script>

<svelte:head>
	{#if setPageTitle}
		<title>{title} · Queen Console Next</title>
	{/if}
</svelte:head>

<section class="space-y-6">
	<header class="space-y-2">
		<div class="flex flex-wrap items-center gap-3">
			{#if level === 1}
				<h1 class="text-3xl font-bold">{title}</h1>
			{:else}
				<h2 class="text-2xl font-bold">{title}</h2>
			{/if}
			<StatusBadge status="pending" label="Planned" />
		</div>
		<p class="max-w-3xl text-base-content/70">{description}</p>
	</header>

	<div class="card bg-base-100 shadow-sm">
		<div class="card-body items-center py-16 text-center">
			<h2 class="card-title text-xl">Migration pending</h2>
			<p class="max-w-xl text-base-content/60">{emptyMessage}</p>
			<div class="card-actions mt-4">
				<a class="btn btn-primary" href="/">Use legacy console</a>
			</div>
		</div>
	</div>
</section>
