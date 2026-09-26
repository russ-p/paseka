<script lang="ts">
	import { base } from '$app/paths';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import { formatTimestamp, reviewDeliveryLabel, taskReviewLabel } from '$lib/format';
	import { reviewDetailPath } from '$lib/navigation';
	import { createReviewStore, type ReviewStore } from '$lib/stores/review.svelte';
	import type { ReviewQueueItem } from '$lib/api/types';

	let { store = createReviewStore() }: { store?: ReviewStore } = $props();

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	const columns: DataColumn<ReviewQueueItem>[] = [
		{
			key: 'proposal',
			label: 'Proposal',
			// A review queue is a list of work waiting on a person, so the row leads
			// with what was asked and the ids stay secondary.
			text: (item) => item.title,
			grow: true
		},
		{
			key: 'bee',
			label: 'Bee',
			text: (item) => item.bee ?? '',
			mono: true,
			searchText: (item) => item.sector ?? ''
		},
		{
			key: 'gate',
			label: 'Gate',
			text: (item) => taskReviewLabel(item.review),
			badge: (item) =>
				item.isFinal
					? { status: 'waiting_review', label: 'final gate' }
					: { status: 'waiting_review', label: taskReviewLabel(item.review) }
		},
		{
			key: 'delivery',
			label: 'Delivery',
			text: (item) => reviewDeliveryLabel(item),
			searchText: (item) => item.delivery ?? '',
			// Only meaningful for a final gate, so it is hidden on the phone rather than
			// repeated as a word on every row.
			secondary: true
		},
		{
			key: 'updated',
			label: 'Waiting',
			text: (item) => formatTimestamp(item.updatedAt),
			searchText: (item) => item.updatedAt ?? ''
		},
		{
			key: 'review',
			label: 'Review',
			text: (item) => `${item.traceId}/${item.taskId}`,
			mono: true,
			href: (item) => reviewDetailPath(base, item.traceId, item.taskId)
		}
	];

</script>

<svelte:head>
	<title>Reviews · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="space-y-1">
		<h1 class="text-3xl font-bold">Reviews</h1>
		<p class="text-base-content/70">
			{store.count === 0
				? 'Nothing is waiting on you. A task joins this queue when it stops at a review gate.'
				: store.count === 1
					? 'One proposal is waiting on you — a task that stopped at a review gate, with the diff its bee left behind.'
					: `${store.count} proposals are waiting on you — tasks that stopped at a review gate, each with the diff its bee left behind.`}
		</p>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-64 w-full"></span>
		</div>
	{:else}
		<DataTable
			label="Review queue"
			{columns}
			rows={store.items}
			rowKey={(item) => `${item.traceId}/${item.taskId}`}
			emptyMessage="No proposals awaiting review."
			filterLabel="Filter the queue"
			filterPlaceholder="proposal, bee, sector, trail, task"
			pageSize={15}
		/>
	{/if}
</div>
