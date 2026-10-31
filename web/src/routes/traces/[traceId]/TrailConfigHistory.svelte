<script lang="ts">
	import Section from '$lib/components/Section.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { formatTimestamp } from '$lib/format';
	import type { TraceStore } from '$lib/stores/trace.svelte';
	import type { ConfigChange } from '$lib/api/types';

	let { store }: { store: TraceStore } = $props();

	const detail = $derived(store.detail);
	const manifest = $derived(detail?.config ?? null);
	/** Newest first: the change the operator is waiting on sits at the top. */
	const history = $derived([...(manifest?.history ?? [])].reverse());

	/** Who did it, from where, and what the server recorded about it. */
	function changeMeta(change: ConfigChange): string {
		return [
			change.actor ? `by ${change.actor}` : '',
			change.source ? `source ${change.source}` : '',
			change.detail ? change.detail : ''
		]
			.filter(Boolean)
			.join(' · ');
	}
</script>

<!-- Its own section, not a section nested in `Configuration`: the configuration
     body hides behind a closed `<details>`, and a child of that summary would
     be unreachable until somebody expands it. The history is read without
     expanding anything, so it stays a sibling — the page's last block. The id
     is kept from the panel, and the title names the section it belongs to:
     beside a collapsed `Configuration`, a bare `Change history` read as the
     tail of a section that was no longer its parent. -->
<Section
	id="trace-config-history"
	title="Configuration History"
	note={history.length > 0 ? String(history.length) : undefined}
	collapsible
	open={false}
>
	{#if history.length === 0}
		<p class="text-sm text-base-content/60">No configuration changes recorded.</p>
	{:else}
		<ul class="space-y-2">
			{#each history as change (change.changeId ?? `${change.revision}-${change.createdAt}`)}
				<li class="rounded-box border border-base-300 px-3 py-2" data-config-change={change.changeId ?? ''}>
					<div class="flex flex-wrap items-start justify-between gap-2">
						<div class="min-w-0">
							<p class="text-sm">
								Revision {change.revision} · {change.source || 'configuration'}
								{#if change.changeId}<span class="font-mono text-xs text-base-content/50"> · {change.changeId}</span>{/if}
							</p>
							{#if changeMeta(change)}
								<p class="text-xs text-base-content/60">{changeMeta(change)}</p>
							{/if}
							{#if change.reason}
								<p class="text-xs text-base-content/60">“{change.reason}”</p>
							{/if}
							{#if change.newConfigRef}
								<p class="truncate font-mono text-xs text-base-content/50">{change.newConfigRef}</p>
							{/if}
							{#if change.blockers?.length}
								<ul class="mt-1 list-inside list-disc text-xs text-warning">
									{#each change.blockers as blocker (blocker)}
										<li>{blocker}</li>
									{/each}
								</ul>
							{/if}
						</div>
						<div class="flex shrink-0 flex-col items-end gap-1">
							<StatusBadge status={change.outcome ?? 'unknown'} label={change.outcome ?? 'unknown'} />
							<span class="text-xs text-base-content/50">{formatTimestamp(change.createdAt)}</span>
						</div>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</Section>
