<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		id,
		title,
		note,
		actions,
		class: extra = '',
		collapsible = false,
		open = true,
		children
	}: {
		/** Test and deep-link hook, as on the topbar panels. */
		id?: string;
		title: string;
		/** Quiet second line in the header: a count, a caveat, a provenance word. */
		note?: string;
		/** Controls on the header row, right of the title. */
		actions?: Snippet;
		/** Extra classes for the block itself, e.g. a grid span. */
		class?: string;
		/** A collapsible section starts closed unless `open` says otherwise. */
		collapsible?: boolean;
		open?: boolean;
		children: Snippet;
	} = $props();
</script>

{#snippet heading()}
	<span class="flex min-w-0 flex-wrap items-baseline gap-x-2">
		<span class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">{title}</span>
		{#if note}
			<span class="text-xs text-base-content/50">{note}</span>
		{/if}
	</span>
{/snippet}

{#snippet controls()}
	{#if actions}
		<!-- A control inside a summary must not toggle the section. -->
		<div class="flex shrink-0 items-center gap-2" role="presentation" onclick={(event) => event.stopPropagation()}>
			{@render actions()}
		</div>
	{/if}
{/snippet}

{#if collapsible}
	<details {id} class="collapse collapse-arrow rounded-box border border-base-300 bg-base-100 {extra}" {open}>
		<summary class="collapse-title min-h-0 px-3 py-2">
			<span class="flex flex-wrap items-center justify-between gap-2 pr-4">
				{@render heading()}
				{@render controls()}
			</span>
		</summary>
		<div class="collapse-content px-3 pb-3">
			{@render children()}
		</div>
	</details>
{:else}
	<section {id} class="rounded-box border border-base-300 bg-base-100 {extra}">
		<header class="flex min-h-8 flex-wrap items-center justify-between gap-2 px-3 py-2">
			<h2 class="min-w-0 text-xs font-semibold tracking-wide text-base-content/60 uppercase">
				{@render heading()}
			</h2>
			{@render controls()}
		</header>
		<div class="px-3 pb-3">
			{@render children()}
		</div>
	</section>
{/if}
