<script lang="ts">
	import Hint from '$lib/components/Hint.svelte';
	import type { Snippet } from 'svelte';

	let {
		id,
		label,
		value,
		hint,
		href,
		class: extra = '',
		children
	}: {
		/** Optional test and deep-link hook, as on the topbar panels. */
		id?: string;
		label: string;
		/** Rendered as the big line unless a content snippet replaces it. */
		value?: string | number;
		/**
		 * Full text for the hover/focus popover, one line per entry — the same
		 * `string[]` shape `MetaRow.hint` and `Hint` use, so a caller with two
		 * lines of explanation does not have to join them into one sentence.
		 */
		hint?: string[];
		href?: string;
		/** Extra classes for the tile itself, e.g. a grid span. */
		class?: string;
		children?: Snippet;
	} = $props();
</script>

<div {id} class={`stat rounded-box border border-base-300 bg-base-100 px-3 py-2 ${extra}`}>
	<div class="flex items-center justify-between gap-2">
		<span class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">{label}</span>
		{#if href}
			<a class="link text-xs text-base-content/50" {href}>open</a>
		{/if}
	</div>
	{#if children}
		<div class="mt-1">{@render children()}</div>
	{:else if hint}
		<Hint lines={hint}>
			<p class="truncate text-xl leading-tight font-bold">{value}</p>
		</Hint>
	{:else}
		<p class="mt-1 text-xl leading-tight font-bold">{value}</p>
	{/if}
</div>
