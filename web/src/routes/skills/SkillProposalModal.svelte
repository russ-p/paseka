<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { getSkillProposal } from '$lib/api/client';
	import { skillProposalStateLabel, skillProposalTimeLabel } from '$lib/format';
	import type { SkillProposalDetail } from '$lib/api/types';

	let {
		open,
		trail,
		name,
		onclose
	}: {
		open: boolean;
		trail: string;
		name: string;
		onclose: () => void;
	} = $props();

	let detail = $state<SkillProposalDetail | null>(null);
	let loading = $state(false);
	let error = $state('');
	/** The proposal the in-flight read belongs to, so a late answer for another row is dropped. */
	let requested = $state('');

	$effect(() => {
		if (!open || !trail || !name) return;
		const key = `${trail}/${name}`;
		requested = key;
		detail = null;
		error = '';
		loading = true;
		void getSkillProposal(trail, name)
			.then((view) => {
				if (requested !== key) return;
				detail = view;
			})
			.catch((cause: unknown) => {
				if (requested !== key) return;
				error = cause instanceof Error ? cause.message : String(cause);
			})
			.finally(() => {
				if (requested === key) loading = false;
			});
	});

	const title = $derived(name || 'Proposal');
	const description = $derived(
		detail
			? `${trail} · ${skillProposalStateLabel(detail)} · proposed ${skillProposalTimeLabel(detail.proposedAt)}`
			: trail
				? `trail ${trail}`
				: undefined
	);
</script>

<Modal {open} {title} {description} size="lg" {onclose}>
	{#if loading}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-3 w-full"></span>
			<span class="skeleton block h-3 w-5/6"></span>
		</div>
	{:else if error}
		<div class="alert alert-error" role="alert"><span>{error}</span></div>
	{:else if detail}
		<div class="space-y-4">
			<div class="space-y-1">
				<h3 class="text-sm font-semibold">Why it was proposed</h3>
				<p class="text-sm text-base-content/70">
					{detail.proposedReason || 'The discovery carried no reason.'}
				</p>
			</div>

			{#if detail.description}
				<div class="space-y-1">
					<h3 class="text-sm font-semibold">Description</h3>
					<p class="text-sm text-base-content/70">{detail.description}</p>
				</div>
			{/if}

			<div class="flex flex-wrap items-center gap-2 text-xs text-base-content/60">
				<StatusBadge
					status={detail.state}
					label={skillProposalStateLabel(detail)}
				/>
				<span class="font-mono">{detail.bundle}</span>
			</div>

			{#if detail.bodyError}
				<!-- A missing bundle is a proposal state, not a failed read: it is
				     reported where the body would have been. -->
				<div class="alert alert-warning" role="alert"><span>{detail.bodyError}</span></div>
			{:else}
				<div class="space-y-1">
					<h3 class="text-sm font-semibold">SKILL.md</h3>
					<pre
						class="overflow-x-auto rounded-box bg-base-200 p-3 font-mono text-xs whitespace-pre-wrap">{detail.body}</pre>
				</div>
			{/if}

			{#if detail.validation.length > 0}
				<div class="alert alert-warning" role="alert">
					<span>Bundle findings: {detail.validation.join('; ')}</span>
				</div>
			{/if}
		</div>
	{/if}

	{#snippet footer()}
		<button type="button" class="btn btn-sm" onclick={onclose}>Close</button>
	{/snippet}
</Modal>
