<script lang="ts">
	import PagePlaceholder from '$lib/components/PagePlaceholder.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import ThemeSelect from '$lib/components/ThemeSelect.svelte';
</script>

<svelte:head>
	<title>Settings · Queen Console Next</title>
</svelte:head>

<section class="space-y-6">
	<header class="space-y-2">
		<div class="flex flex-wrap items-center gap-3">
			<h1 class="text-3xl font-bold">Settings</h1>
			<StatusBadge status="pending" label="Partial" />
		</div>
		<p class="max-w-3xl text-base-content/70">
			Theme selection is available now. Remaining settings migrate after the shell and feature parity.
		</p>
	</header>

	<div class="card bg-base-100 shadow-sm">
		<div class="card-body">
			<ThemeSelect />
		</div>
	</div>

	<PagePlaceholder
		title="Console settings"
		description="Runtime endpoints, adapters, and notification preferences will be configured here."
		emptyMessage="Only theming is available in the preview. Use the legacy console for existing configuration."
		level={2}
		setPageTitle={false}
	/>
</section>
