<script lang="ts">
	import Hint from '$lib/components/Hint.svelte';
	import type { Snippet } from 'svelte';

	let {
		title,
		meta,
		detail,
		side,
		href,
		dataKey
	}: {
		/** Headline; falls back to the id when the author gave no title. */
		title: string;
		/** Monospace second line: the identifier the operator matches against. */
		meta?: string;
		/** Quiet third line: a time, a token spend, a path. */
		detail?: string;
		/** Badges on the right — the row's state. */
		side?: Snippet;
		/** Makes the headline a link. Omitted while the target route is a placeholder. */
		href?: string;
		/** Test and deep-link hook carrying the row's identifier. */
		dataKey?: string;
	} = $props();
</script>

<li
	class="detail-row rounded-box border border-base-300 bg-base-100 px-3 py-2"
	data-key={dataKey}
>
	<div class="flex items-start justify-between gap-3">
		<div class="min-w-0">
			{#if href}
				<a class="link block truncate text-sm font-medium" {href}>{title}</a>
			{:else}
				<p class="truncate text-sm font-medium">{title}</p>
			{/if}
			{#if meta}
				<p class="truncate font-mono text-xs text-base-content/50">{meta}</p>
			{/if}
			{#if detail}
				<Hint lines={[detail]}>
					<p class="truncate text-xs text-base-content/50">{detail}</p>
				</Hint>
			{/if}
		</div>
		{#if side}
			<div class="flex shrink-0 flex-wrap items-center justify-end gap-1">{@render side()}</div>
		{/if}
	</div>
</li>
