<script lang="ts">
	import { base } from '$app/paths';
	import { Plus } from 'lucide-svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { createTaskStore, type TaskStore } from '$lib/stores/task.svelte';
	import {
	formatTimestamp,
	taskCardShowsId,
	taskMeta,
	taskRowMeta,
	taskStatusLabel,
	taskTitle
} from '$lib/format';
	import { taskDetailPath } from '$lib/navigation';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import TaskCreateDrawer from './TaskCreateDrawer.svelte';
	import type { CreateTaskResult, TaskListItem } from '$lib/api/types';

	let {
		store = createTaskStore(),
		toasts = toastStore
	}: { store?: TaskStore; toasts?: ToastStore } = $props();

	let creating = $state(false);

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	/** A card links to its own page, so a task can be shared and Back behaves. */
	function href(task: TaskListItem): string {
		return taskDetailPath(base, task.traceId, task.taskId);
	}

	/**
	 * The board is a grid of status columns rather than one wide scroller: seven
	 * lifecycle statuses side by side would put the last two off the right edge on
	 * a laptop and entirely out of reach on a phone. Wrapping keeps every column
	 * reachable, and the server's group order — ready, running, waiting_review,
	 * planned, blocked, failed, completed — is the pipeline read left to right.
	 */
	const visibleGroups = $derived(store.groups.filter((group) => group.tasks.length > 0));

	function oncreated(result: CreateTaskResult): void {
		toasts.push(
			'success',
			result.autorun
				? `Task ${result.taskId} created and started`
				: `Task ${result.taskId} created`
		);
		void store.refresh();
	}
</script>

<svelte:head>
	<title>Tasks · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0 space-y-1">
			<h1 class="text-3xl font-bold">Tasks</h1>
			<p class="text-base-content/70">
				Every task the colony's ledger knows, one column per lifecycle status — a task moves
				right as the bees get to it and stops at the review it waits on.
			</p>
		</div>
		<button id="task-create-open" type="button" class="btn btn-primary btn-sm" onclick={() => (creating = true)}>
			<Plus class="h-4 w-4" strokeWidth={2.5} />
			New task
		</button>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.showSkeletons}
		<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3" aria-busy="true">
			{#each [0, 1, 2] as slot (slot)}
				<div class="space-y-2">
					<span class="skeleton block h-4 w-1/3"></span>
					<span class="skeleton block h-24 w-full"></span>
				</div>
			{/each}
		</div>
	{:else if visibleGroups.length === 0}
		<div class="card bg-base-100 shadow-sm">
			<div class="card-body items-center py-16 text-center">
				<h2 class="card-title text-xl">No tasks</h2>
				<p class="text-base-content/70">
					The board is empty. A task arrives when a planner publishes
					<code class="font-mono">task.plan</code>, or when you create one here.
				</p>
				<button type="button" class="btn btn-primary btn-sm mt-2" onclick={() => (creating = true)}>
					New task
				</button>
			</div>
		</div>
	{:else}
		<div class="grid items-start gap-4 md:grid-cols-2 xl:grid-cols-3">
			{#each visibleGroups as group (group.status)}
				{@const count = store.counts[group.status] ?? group.tasks.length}
				<section class="rounded-box border border-base-300" aria-label={`${taskStatusLabel(group.status)} tasks`}>
					<!-- The topbar's panel header, reused: the status names the column in
					     the same uppercase small-caps the runtime and bees panels use, and
					     the count is the badge on the right. The badge carries the status
					     tone, so moving the status word out of a badge and into the title
					     costs the colour nothing. -->
					<header class="flex min-h-8 items-center justify-between gap-2 border-b border-base-300 px-3 py-2">
						<span class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">
							{taskStatusLabel(group.status)}
						</span>
						<StatusBadge status={group.status} label={String(count)} />
					</header>
					<!-- Each column scrolls inside its own bounded box. This colony keeps 29
					     completed tasks beside one ready one, and an unbounded column turns
					     the board into a three-thousand-pixel ribbon of history with the
					     work buried at the top. The count in the header stays visible, so a
					     column that scrolls still says how much it holds. -->
					<ul class="max-h-[30rem] space-y-2 overflow-y-auto p-2">
						<!-- Both ids, because the board is colony-wide: `_review` is the
						     default rework task's id, and five trails each have one. Keyed
						     on the task id alone the board threw `each_key_duplicate` and
						     rendered nothing at all. -->
						{#each group.tasks as task (`${task.traceId}/${task.taskId}`)}
							<li>
								<a
									class="block space-y-1 rounded-box bg-base-100 p-3 shadow-sm transition-colors hover:bg-base-200"
									href={href(task)}
								>
									<span class="block text-sm font-medium">{taskTitle(task)}</span>
									{#if taskCardShowsId(task)}
										<span class="block font-mono text-xs text-base-content/60">
											{taskMeta(task)}
										</span>
									{/if}
									<span class="block text-xs text-base-content/60">{taskRowMeta(task)}</span>
									<span class="flex flex-wrap items-center gap-1 pt-1 text-xs text-base-content/50">
										{#if task.sector}
											<span class="badge badge-ghost badge-sm">{task.sector}</span>
										{/if}
										{#if task.review && task.review !== 'none'}
											<span class="badge badge-ghost badge-sm">{task.review} review</span>
										{/if}
										{#if task.dependsOn?.length}
											<span class="badge badge-ghost badge-sm" title={`Depends on ${task.dependsOn.join(', ')}`}>
													after {task.dependsOn.join(', ')}
												</span>
										{/if}
										{#if task.canStart}
											<!-- The server's own eligibility answer, so a card never offers a
											     start the ledger would refuse a moment later. -->
											<span class="badge badge-success badge-sm badge-outline">startable</span>
										{/if}
										{#if task.canRetry}
											<span class="badge badge-warning badge-sm badge-outline">retryable</span>
										{/if}
										{#if task.updatedAt}
											<span class="ml-auto">{formatTimestamp(task.updatedAt)}</span>
										{/if}
									</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/each}
		</div>
	{/if}
</div>

<TaskCreateDrawer open={creating} onclose={() => (creating = false)} oncreated={oncreated} />
