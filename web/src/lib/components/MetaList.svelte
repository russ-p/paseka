<script lang="ts">
	import { onDestroy } from 'svelte';
	import { Check, Copy } from 'lucide-svelte';
	import Hint from '$lib/components/Hint.svelte';
	import { copyText } from '$lib/clipboard';
	import type { MetaRow } from '$lib/format';

	let {
		rows,
		label,
		columns = 2
	}: {
		rows: MetaRow[];
		/** Accessible name for the list, since a `dl` carries no heading of its own. */
		label: string;
		/** Grid columns above 768px; one column keeps long values readable on a phone. */
		columns?: 1 | 2 | 3;
	} = $props();

	const columnClass = { 1: '', 2: 'sm:grid-cols-2', 3: 'sm:grid-cols-3' } as const;

	/** Label of the row whose copy landed, so only that button confirms. */
	let copied = $state('');
	let copyTimer: ReturnType<typeof setTimeout> | undefined;

	async function copy(row: MetaRow): Promise<void> {
		if (!(await copyText(row.value))) return;
		copied = row.label;
		if (copyTimer) clearTimeout(copyTimer);
		copyTimer = setTimeout(() => (copied = ''), 1500);
	}

	onDestroy(() => {
		if (copyTimer) clearTimeout(copyTimer);
	});
</script>

<dl class="grid grid-cols-1 gap-x-6 {columnClass[columns]}" aria-label={label}>
	{#each rows as row (row.label)}
		<div class="flex items-baseline justify-between gap-3 border-b border-base-200 py-1.5 last:border-b-0">
			<dt class="shrink-0 text-xs text-base-content/60">{row.label}</dt>
			<dd class="flex min-w-0 items-baseline justify-end gap-1 text-right">
				{#snippet value()}
					{#if row.href}
						<a
							class="link min-w-0 flex-1 truncate {row.mono ? 'font-mono' : ''}"
							href={row.href}
							target="_blank"
							rel="noopener noreferrer">{row.value}</a
						>
					{:else}
						<!-- `min-w-0` is what lets this shrink. A flex item defaults to
						     `min-width: auto`, so without it a long path refuses to
						     truncate and overruns its own label. -->
						<span class="min-w-0 flex-1 truncate {row.mono ? 'font-mono' : ''}">{row.value}</span>
					{/if}
				{/snippet}
				{#if row.hint?.length}
					<Hint lines={row.hint}><span class="block min-w-0 truncate">{@render value()}</span></Hint>
				{:else}
					{@render value()}
				{/if}
				{#if row.copy}
					{@const done = copied === row.label}
					<button
						type="button"
						class="btn btn-ghost btn-xs shrink-0"
						aria-label={done ? `${row.label} copied` : `Copy ${row.label.toLowerCase()}`}
						title={done ? 'Copied' : 'Copy'}
						onclick={() => void copy(row)}
					>
						{#if done}
							<Check class="h-3.5 w-3.5 text-success" strokeWidth={2.5} />
						{:else}
							<Copy class="h-3.5 w-3.5" strokeWidth={2} />
						{/if}
					</button>
				{/if}
			</dd>
		</div>
	{/each}
</dl>
