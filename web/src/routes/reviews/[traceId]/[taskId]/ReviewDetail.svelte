<script lang="ts">
	import { base } from '$app/paths';
	import { Maximize2 } from 'lucide-svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import ReviewActions from '$lib/components/ReviewActions.svelte';
	import Section from '$lib/components/Section.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { formatTimestamp, taskReviewLabel } from '$lib/format';
	import { buildMergeDiffFiles } from '$lib/diff';
	import {
		consolePath,
		reviewDetailPath,
		reviewPreviewPath,
		taskDetailPath,
		traceDetailPath,
		traceTimelinePath
	} from '$lib/navigation';
	import { createReviewStore, type ReviewStore } from '$lib/stores/review.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import type { MetaRow } from '$lib/format';

	let {
		store = createReviewStore(),
		traceId = '',
		taskId = '',
		toasts = toastStore
	}: {
		store?: ReviewStore;
		traceId?: string;
		taskId?: string;
		toasts?: ToastStore;
	} = $props();

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	$effect(() => {
		if (traceId && taskId) void store.select(traceId, taskId);
	});

	const item = $derived(store.current);
	const diff = $derived(store.diff);
	/** Parsed only to decide whether a preview is worth linking to. */
	const files = $derived(diff ? buildMergeDiffFiles(diff) : []);
	const canAct = $derived(item?.canApprove === true && item?.canReject === true);
	const prUrl = $derived(item?.pullRequest?.url);

	const metaRows = $derived<MetaRow[]>(
		item
			? [
					{
						label: 'Trail',
						value: item.traceId,
						mono: true,
						href: traceDetailPath(base, item.traceId)
					},
					{ label: 'Task', value: item.taskId, mono: true, copy: true, hint: [item.taskId] },
					...reviewGateRows(item)
				]
			: []
	);

	/**
	 * The gate and its consequences, in the order a reviewer asks: what is stopping
	 * it, what approving will do, and whether a rework is already running.
	 */
	function reviewGateRows(current: NonNullable<typeof item>): MetaRow[] {
		const rows: MetaRow[] = [];
		if (current.bee) rows.push({ label: 'Bee', value: current.bee, mono: true });
		if (current.sector) rows.push({ label: 'Sector', value: current.sector, mono: true });
		rows.push({ label: 'Review', value: taskReviewLabel(current.review) });
		if (current.isFinal) {
			rows.push({
				label: 'Gate',
				value: 'final merge gate',
				hint: ['The last work to leave this trail needs your sign-off, and the trail waits on it.']
			});
			rows.push({ label: 'Delivery', value: deliveryOf(current) });
		}
		if (current.runCount > 0) rows.push({ label: 'Runs', value: String(current.runCount) });
		rows.push({
			label: 'Updated',
			value: formatTimestamp(current.updatedAt),
			hint: [formatTimestamp(current.updatedAt), current.updatedAt ?? '']
		});
		if (current.reworkTaskId) {
			rows.push({
				label: 'Rework',
				value: `${current.reworkTaskId} (${current.reworkStatus ?? 'in flight'})`,
				hint: [
					`Rework ${current.reworkTaskId} is ${current.reworkStatus ?? 'in flight'}. Request changes is disabled until it finishes.`
				]
			});
		}
		return rows;
	}

	function deliveryOf(current: NonNullable<typeof item>): string {
		if (prUrl || current.delivery === 'pull_request') return 'pull request';
		return 'local merge';
	}

	function settled(outcome: { tone: 'success' | 'warning'; message: string }): void {
		toasts.push(outcome.tone, outcome.message);
		void store.refresh();
	}
</script>

<svelte:head>
	<title>{item ? `Review ${item.title}` : 'Review'} · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 space-y-1">
			<a class="link text-xs text-base-content/60" href={consolePath(base, '/reviews')}>← Reviews</a>
			<div class="flex flex-wrap items-center gap-2">
				<h1 class="text-2xl font-bold">{item?.title ?? taskId ?? 'Review'}</h1>
				{#if item}
					<StatusBadge status="waiting_review" label="waiting review" />
				{/if}
			</div>
			{#if item}
				<p class="text-base-content/70">
					Waiting since {formatTimestamp(item.updatedAt)}
					{#if item.bee}— {item.bee}{/if}
				</p>
			{/if}
		</div>

		<div class="flex flex-wrap items-center gap-2">
			<a class="btn btn-sm" href={taskDetailPath(base, traceId, taskId)}>Task</a>
			<a class="btn btn-sm" href={traceTimelinePath(base, traceId)}>Timeline</a>
		</div>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.detailError && !item}
		<div class="card bg-base-100 shadow-sm">
			<div class="card-body items-center py-16 text-center">
				<h2 class="card-title text-xl">Proposal not found</h2>
				<p class="text-base-content/70">
					No task <span class="font-mono">{taskId}</span> on trail
					<span class="font-mono">{traceId}</span>. It may already have been approved, or the ids may
					be mistyped.
				</p>
				<a class="btn btn-sm mt-2" href={consolePath(base, '/reviews')}>All reviews</a>
			</div>
		</div>
	{:else if store.detailLoading && !item}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-8 w-1/2"></span>
			<span class="skeleton block h-40 w-full"></span>
		</div>
	{:else if item}
		<Section id="review-identity" title="Proposal">
			<MetaList rows={metaRows} label="Proposal identity" columns={2} />
			{#if prUrl}
				<p class="mt-3 text-sm">
					Pull request opened:
					<a class="link" href={prUrl} target="_blank" rel="noopener noreferrer">{prUrl}</a>
				</p>
			{/if}
		</Section>

		{#if item.summary}
			<Section id="review-summary" title="Summary" note="as the bee wrote it" collapsible open>
				<pre
					class="max-h-96 overflow-auto rounded-box bg-base-200/50 p-3 font-mono text-xs whitespace-pre-wrap">{item.summary}</pre
				>
			</Section>
		{/if}

		<!-- The trail summary is the merge commit body the bee would have written, so
		     a reviewer reads it before approving rather than discovering it afterwards. -->
		{#if item.isFinal && item.traceSummary}
			<Section
				id="review-trail-summary"
				title="Trail summary"
				note="becomes the merge commit body"
			>
				<p class="text-sm whitespace-pre-wrap">{item.traceSummary}</p>
			</Section>
		{/if}

		{#if item.isFinal}
			<Section
				id="review-diff"
				title="Changes"
				note={store.diffLoading ? 'loading' : `${files.length} file${files.length === 1 ? '' : 's'}`}
			>
				{#if store.diffError}
					<div class="alert alert-error" role="alert"><span>{store.diffError}</span></div>
				{:else if store.diffLoading}
					<div class="space-y-2" aria-busy="true">
						<span class="skeleton block h-3 w-1/3"></span>
						<span class="skeleton block h-48 w-full"></span>
					</div>
				{:else if diff?.missingWorktree}
					<p class="text-base-content/70">
						No branch for this trail on this machine, so there is nothing to preview.
					</p>
				{:else if diff?.empty || !diff?.diff}
					<p class="text-base-content/70">
						No changes between <span class="font-mono">{diff?.defaultBranch}</span> and
						<span class="font-mono">{diff?.branch}</span>. Nothing to merge.
					</p>
				{:else}
					<div class="space-y-3">
						{#if (diff?.originBehindCount ?? 0) > 0}
							<div class="alert alert-warning" role="alert">
								<span>
									The local <span class="font-mono">{diff?.defaultBranch}</span> is
									{diff?.originBehindCount} commit(s) behind origin. A merge here may conflict.
								</span>
							</div>
						{/if}
						{#if diff?.truncated}
							<div class="alert alert-warning" role="alert">
								<span>
									Diff truncated at the server’s size cap — open the worktree locally for the
									full patch.
								</span>
							</div>
						{/if}
						<div class="flex flex-wrap items-center gap-2">
							<a
								class="btn btn-sm"
								href={reviewPreviewPath(base, traceId, taskId)}
							>
								<Maximize2 class="h-4 w-4" strokeWidth={2.5} />
								Open merge preview
							</a>
							<span class="text-xs text-base-content/50">
								{diff?.branch} against {diff?.defaultBranch} at
								<span class="font-mono">{diff?.headSha?.slice(0, 7)}</span>
							</span>
						</div>
						{#if diff?.stat}
							<pre
								class="max-h-48 overflow-auto rounded-box bg-base-200/50 p-3 font-mono text-xs">{diff.stat}</pre
							>
						{/if}
					</div>
				{/if}
			</Section>
		{/if}

		{#if canAct}
			<Section id="review-actions" title="Decide">
				<ReviewActions task={item} onsettled={settled} />
			</Section>
		{/if}
	{/if}
</div>
