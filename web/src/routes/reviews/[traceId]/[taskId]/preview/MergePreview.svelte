<script lang="ts">
	import { base } from '$app/paths';
	import CommentThreads from '$lib/components/CommentThreads.svelte';
	import DiffViewer from '$lib/components/DiffViewer.svelte';
	import type { DiffAnchor } from '$lib/diff';
	import { consolePath, reviewDetailPath } from '$lib/navigation';
	import { createReviewStore, type ReviewStore } from '$lib/stores/review.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';

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

	let comments = $state<ReturnType<typeof CommentThreads> | null>(null);

	const item = $derived(store.current);
	const diff = $derived(store.diff);

	/**
	 * The panel is the point of this screen, so the diff goes to the left and the
	 * notes to the right on a wide viewport and stack on a phone. The comment state
	 * lives in the panel and the body only reports which line was clicked.
	 */
	function pick(anchor: DiffAnchor): void {
		comments?.anchorOn(anchor);
	}

	function settled(outcome: { tone: 'success' | 'warning'; message: string }): void {
		toasts.push(outcome.tone, outcome.message);
		void store.refresh();
	}
</script>

<svelte:head>
	<title>Merge preview · Queen Console Next</title>
</svelte:head>

<div class="space-y-4">
	<header class="flex flex-wrap items-center justify-between gap-3">
		<div class="min-w-0 space-y-1">
			<a
				class="link text-xs text-base-content/60"
				href={reviewDetailPath(base, traceId, taskId)}>← Back to review</a
			>
			<h1 class="text-2xl font-bold">Merge preview</h1>
			{#if diff}
				<p class="text-base-content/70">
					<span class="font-mono">{diff.branch}</span> against
					<span class="font-mono">{diff.defaultBranch}</span> at
					<span class="font-mono">{diff.headSha?.slice(0, 7)}</span>
				</p>
			{/if}
		</div>
		<a class="btn btn-sm" href={consolePath(base, '/reviews')}>All reviews</a>
	</header>

	{#if store.diffError}
		<div class="alert alert-error" role="alert"><span>{store.diffError}</span></div>
	{:else if store.diffLoading}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-96 w-full"></span>
		</div>
	{:else if diff}
		<div class="grid gap-4 xl:grid-cols-[1fr_20rem]">
			<DiffViewer {diff} onpickline={pick} />
			<aside class="xl:sticky xl:top-4 xl:self-start">
				{#if item}
					<CommentThreads
						bind:this={comments}
						task={item}
						headSha={diff.headSha ?? ''}
						onsettled={settled}
					/>
				{/if}
			</aside>
		</div>
	{/if}
</div>
