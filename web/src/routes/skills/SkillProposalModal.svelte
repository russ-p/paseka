<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { skillProposalStateLabel, skillProposalTimeLabel } from '$lib/format';
	import type { SkillApprovalPlan, SkillProposalDetail } from '$lib/api/types';
	import type { SkillsStore } from '$lib/stores/skills.svelte';

	type View = 'detail' | 'approve' | 'reject';

	let {
		open,
		trail,
		name,
		store,
		onclose
	}: {
		open: boolean;
		trail: string;
		name: string;
		store: SkillsStore;
		onclose: () => void;
	} = $props();

	let detail = $state<SkillProposalDetail | null>(null);
	let loading = $state(false);
	let error = $state('');
	/** The proposal the in-flight read belongs to, so a late answer for another row is dropped. */
	let requested = $state('');

	/** The panel the dialog is showing: the review, the install plan, or the refusal. */
	let view = $state<View>('detail');
	let plan = $state<SkillApprovalPlan | null>(null);
	let planLoading = $state(false);
	let planError = $state('');
	let actionError = $state('');
	let reason = $state('');

	function message(cause: unknown): string {
		return cause instanceof Error ? cause.message : String(cause);
	}

	$effect(() => {
		if (!open || !trail || !name) return;
		const key = `${trail}/${name}`;
		requested = key;
		detail = null;
		error = '';
		loading = true;
		view = 'detail';
		plan = null;
		planLoading = false;
		planError = '';
		actionError = '';
		reason = '';
		void store
			.proposal(trail, name)
			.then((result) => {
				if (requested !== key) return;
				detail = result;
			})
			.catch((cause: unknown) => {
				if (requested !== key) return;
				error = message(cause);
			})
			.finally(() => {
				if (requested === key) loading = false;
			});
	});

	/**
	 * Only a bundle that could still be decided shows the controls. An installed
	 * or refused row is a decision already taken, and a missing or invalid bundle
	 * has nothing to install or refuse.
	 */
	const canDecide = $derived(
		detail !== null &&
			(detail.state === 'pending' || detail.state === 'trialled') &&
			detail.valid &&
			!detail.bundleMissing
	);

	const title = $derived(
		view === 'approve'
			? `Install ${name || 'proposal'}`
			: view === 'reject'
				? `Refuse ${name || 'proposal'}`
				: name || 'Proposal'
	);
	const description = $derived(
		view === 'approve'
			? `from ${trail} into the store`
			: detail
				? `${trail} · ${skillProposalStateLabel(detail)} · proposed ${skillProposalTimeLabel(detail.proposedAt)}`
				: trail
					? `trail ${trail}`
					: undefined
	);

	async function showApprovalPlan(): Promise<void> {
		view = 'approve';
		plan = null;
		planError = '';
		actionError = '';
		planLoading = true;
		try {
			plan = await store.plan(trail, name);
		} catch (cause) {
			planError = message(cause);
		} finally {
			planLoading = false;
		}
	}

	function showRefusal(): void {
		view = 'reject';
		actionError = '';
		reason = '';
	}

	function backToDetail(): void {
		view = 'detail';
		actionError = '';
	}

	async function confirmApproval(): Promise<void> {
		actionError = '';
		try {
			await store.approve(trail, name);
			onclose();
		} catch (cause) {
			// The server's reason is the whole message: a failed decision wrote
			// nothing, and the dialog stays open showing the plan and the failure.
			actionError = message(cause);
		}
	}

	async function confirmRefusal(): Promise<void> {
		actionError = '';
		try {
			await store.reject(trail, name, reason.trim());
			onclose();
		} catch (cause) {
			actionError = message(cause);
		}
	}
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
		{#if view === 'approve'}
			<div class="space-y-4">
				{#if planLoading}
					<div class="space-y-2" aria-busy="true">
						<span class="skeleton block h-3 w-1/3"></span>
						<span class="skeleton block h-3 w-full"></span>
						<span class="skeleton block h-3 w-2/3"></span>
					</div>
				{:else if planError}
					<div class="alert alert-error" role="alert"><span>{planError}</span></div>
				{:else if plan}
					<!-- The confirmation names the same facts as the shell's plan: the
					     skill, its source, the destination, every file, and what the
					     working tree will hold afterwards. -->
					<dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
						<dt class="font-semibold">Skill</dt>
						<dd class="font-mono">{plan.name}</dd>
						<dt class="font-semibold">Source trail</dt>
						<dd class="font-mono">{plan.trail}</dd>
						<dt class="font-semibold">Destination</dt>
						<dd>
							<span class="font-mono">{plan.destination}</span>
							<span class="text-base-content/70"> · {plan.target}</span>
						</dd>
						<dt class="font-semibold">Procedure</dt>
						<dd class="font-mono break-all">{plan.procedureHash}</dd>
					</dl>

					<div class="alert alert-warning" role="note">
						<span>{plan.workingTree}</span>
					</div>
					{#if plan.replaced}
						<div class="alert alert-warning" role="alert">
							<span>The store already holds this name; confirming replaces that directory as a unit.</span>
						</div>
					{/if}
					{#if plan.disabled}
						<div class="alert alert-warning" role="alert">
							<span>
								The existing disable record
								{plan.disableReason ? `(${plan.disableReason}) ` : ''}is kept, so the skill
								stays disabled after installation.
							</span>
						</div>
					{/if}

					<div class="space-y-1">
						<h3 class="text-sm font-semibold">Files</h3>
						<ul class="space-y-1">
							{#each plan.files as file (file)}
								<li class="font-mono text-xs">{file}</li>
							{/each}
						</ul>
					</div>

					{#if actionError}
						<div class="alert alert-error" role="alert"><span>{actionError}</span></div>
					{/if}
				{/if}
			</div>
		{:else if view === 'reject'}
			<div class="space-y-4">
				<p class="text-sm text-base-content/70">
					Refusing <span class="font-mono">{detail.name}</span> from
					<span class="font-mono">{detail.trail}</span> is terminal for this bundle. It
					cannot be approved or trialled afterwards. The decision is machine-local and
					disappears with the trail; nothing is committed.
				</p>
				<label class="form-control w-full">
					<span class="label-text">Reason (optional)</span>
					<textarea
						class="textarea textarea-bordered mt-1 w-full"
						rows="3"
						placeholder="Why this proposal is being turned down"
						bind:value={reason}
					></textarea>
				</label>
				{#if actionError}
					<div class="alert alert-error" role="alert"><span>{actionError}</span></div>
				{/if}
			</div>
		{:else}
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
					<StatusBadge status={detail.state} label={skillProposalStateLabel(detail)} />
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

				{#if actionError}
					<div class="alert alert-error" role="alert"><span>{actionError}</span></div>
				{/if}
			</div>
		{/if}
	{/if}

	{#snippet footer()}
		{#if view === 'approve'}
			<button type="button" class="btn btn-sm" disabled={store.mutating} onclick={backToDetail}>
				Cancel
			</button>
			<button
				type="button"
				class="btn btn-sm btn-primary"
				disabled={planLoading || plan === null || store.mutating}
				onclick={() => void confirmApproval()}
			>
				{store.mutating ? 'Installing…' : 'Install skill'}
			</button>
		{:else if view === 'reject'}
			<button type="button" class="btn btn-sm" disabled={store.mutating} onclick={backToDetail}>
				Cancel
			</button>
			<button
				type="button"
				class="btn btn-sm btn-error"
				disabled={store.mutating}
				onclick={() => void confirmRefusal()}
			>
				{store.mutating ? 'Refusing…' : 'Refuse proposal'}
			</button>
		{:else}
			<button type="button" class="btn btn-sm" onclick={onclose}>Close</button>
			{#if canDecide}
				<button
					type="button"
					class="btn btn-sm btn-error"
					disabled={store.mutating}
					onclick={showRefusal}
				>
					Refuse
				</button>
				<button
					type="button"
					class="btn btn-sm btn-primary"
					disabled={store.mutating}
					onclick={() => void showApprovalPlan()}
				>
					Approve
				</button>
			{/if}
		{/if}
	{/snippet}
</Modal>
