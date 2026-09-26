<script lang="ts">
	import { base } from '$app/paths';
	import { Play, RotateCcw } from 'lucide-svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import Section from '$lib/components/Section.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { retryTask, startTask } from '$lib/api/client';
	import {
		formatTimestamp,
		runStateLabel,
		taskIdentityRows,
		taskRunMeta,
		taskStatusLabel,
		taskTitle
	} from '$lib/format';
	import { consolePath, runDetailPath, taskDetailPath, traceDetailPath } from '$lib/navigation';
	import { createTaskStore, type TaskStore } from '$lib/stores/task.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import ReviewActions from '$lib/components/ReviewActions.svelte';

	let {
		store = createTaskStore(),
		traceId = '',
		taskId = '',
		toasts = toastStore
	}: {
		store?: TaskStore;
		traceId?: string;
		taskId?: string;
		toasts?: ToastStore;
	} = $props();

	let acting = $state<'start' | 'retry' | null>(null);
	let actionError = $state('');

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	$effect(() => {
		if (traceId && taskId) void store.select(traceId, taskId);
	});

	const task = $derived(store.current);
	const identityRows = $derived(taskIdentityRows(task, (id) => traceDetailPath(base, id)));

	/**
	 * A task has no `canStart` of its own once the detail is in hand — the board
	 * row's answer can be minutes old — so the button re-reads the row rather than
	 * re-deciding the rule, and reports the server's refusal as itself.
	 */
	/**
	 * The server answers `null` for a task no run has touched, and Go marshals a nil
	 * slice that way, so the list is normalised once here rather than at each of the
	 * three places that ask it how long it is.
	 */
	const linkedRuns = $derived(task?.runs ?? []);

	const row = $derived(store.row);
	/**
	 * The status comes from the board row when there is one. The detail is read once
	 * and the board is polled every five seconds, so a task approved in another tab
	 * would keep announcing itself as waiting for review here for as long as the page
	 * stayed open — the badge would disagree with the board it links back to.
	 */
	const status = $derived(row?.status ?? task?.status ?? '');
	const canStart = $derived(row?.canStart ?? false);
	const canRetry = $derived(row?.canRetry ?? false);
	const reviewable = $derived(row?.canApprove === true || row?.canReject === true);

	async function act(action: 'start' | 'retry'): Promise<void> {
		if (!task) return;
		acting = action;
		actionError = '';
		try {
			const result =
				action === 'start'
					? await startTask(task.traceId, task.taskId)
					: await retryTask(task.traceId, task.taskId);
			toasts.push(
				'success',
				result.message ?? (action === 'start' ? 'Published task.ready' : 'Published task.ready to retry')
			);
			await store.refresh();
		} catch (cause) {
			actionError = cause instanceof Error ? cause.message : String(cause);
		} finally {
			acting = null;
		}
	}

	function settled(outcome: { tone: 'success' | 'warning'; message: string }): void {
		toasts.push(outcome.tone, outcome.message);
		void store.refresh();
		void store.select(task?.traceId ?? '', task?.taskId ?? '');
	}
</script>

<svelte:head>
	<title>Task {taskId || ''} · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="min-w-0 space-y-1">
		<a class="link text-xs text-base-content/60" href={consolePath(base, '/tasks')}>← Tasks</a>
		<div class="flex flex-wrap items-center gap-2">
			<h1 class="text-2xl font-bold">{task ? taskTitle(task) : taskId || 'Task'}</h1>
			{#if task}
				<StatusBadge status={status} label={taskStatusLabel(status)} />
				{#if row?.review && row.review !== 'none'}
					<span class="badge badge-ghost badge-sm">{row.review} review</span>
				{/if}
			{/if}
		</div>
		{#if task}
			<p class="text-base-content/70">
				<span class="font-mono">{task.taskId}</span>
				{#if task.bee}for {task.bee}{/if}
				{#if task.sector}in {task.sector}{/if}
			</p>
		{/if}
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}
	{#if actionError}
		<div class="alert alert-error" role="alert"><span>{actionError}</span></div>
	{/if}

	{#if store.detailError && !task}
		<div class="card bg-base-100 shadow-sm">
			<div class="card-body items-center py-16 text-center">
				<h2 class="card-title text-xl">Task not found</h2>
				<p class="text-base-content/70">
					No task <span class="font-mono">{taskId}</span> on trail
					<span class="font-mono">{traceId}</span>. It may have been removed from the ledger, or the
					ids may be mistyped.
				</p>
				<a class="btn btn-sm mt-2" href={consolePath(base, '/tasks')}>All tasks</a>
			</div>
		</div>
	{:else if store.detailLoading && !task}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-8 w-1/2"></span>
			<span class="skeleton block h-40 w-full"></span>
		</div>
	{:else if task}
		<!-- Start and Retry are the two things an operator does to a task that is
		     sitting still, so they sit above the detail rather than inside it, and
		     they appear only when the server says the task is eligible. -->
		{#if canStart || canRetry}
			<div class="flex flex-wrap items-center gap-2">
				{#if canStart}
					<button
						id="task-start"
						type="button"
						class="btn btn-primary btn-sm"
						disabled={acting !== null}
						onclick={() => void act('start')}
					>
						<Play class="h-4 w-4" strokeWidth={2.5} />
						{acting === 'start' ? 'Starting…' : 'Start'}
					</button>
				{/if}
				{#if canRetry}
					<button
						id="task-retry"
						type="button"
						class="btn btn-sm"
						disabled={acting !== null}
						onclick={() => void act('retry')}
					>
						<RotateCcw class="h-4 w-4" strokeWidth={2.5} />
						{acting === 'retry' ? 'Retrying…' : 'Retry'}
					</button>
				{/if}
				<span class="text-xs text-base-content/50">
					Publishes <code class="font-mono">task.ready</code> so a dispatcher picks the task up.
				</span>
			</div>
		{/if}

		<Section id="task-identity" title="Task">
			<MetaList rows={identityRows} label="Task identity" columns={2} />
		</Section>

		<!-- The body is the literal prompt, often long, and reading a task is about
		     what it did — so it is folded, like the run's. -->
		{#if task.body}
			<Section
				id="task-body"
				title="Body"
				note="what the bee is handed"
				collapsible
				open={false}
			>
				<pre
					class="max-h-96 overflow-auto rounded-box bg-base-200/50 p-3 font-mono text-xs whitespace-pre-wrap">{task.body}</pre
				>
			</Section>
		{/if}

		{#if task.summary}
			<Section id="task-summary" title="Summary" note="as the bee wrote it" collapsible open={false}>
				<pre
					class="max-h-96 overflow-auto rounded-box bg-base-200/50 p-3 font-mono text-xs whitespace-pre-wrap">{task.summary}</pre
				>
			</Section>
		{/if}

		{#if task.traceSummary}
			<Section
				id="task-trace-summary"
				title="Trail summary"
				note="the whole trail, at the final gate"
			>
				<p class="text-sm whitespace-pre-wrap">{task.traceSummary}</p>
			</Section>
		{/if}

		<Section
			id="task-runs"
			title="Linked runs"
			note={linkedRuns.length > 0 ? `${linkedRuns.length} recorded` : undefined}
		>
			{#if linkedRuns.length === 0}
				<p class="text-base-content/70">
					No runs yet. A task's runs appear once a bee picks it up and the dispatcher starts a
					run for it.
				</p>
			{:else}
				<ul class="space-y-2" aria-label="Linked runs">
					{#each linkedRuns as run (run.agentId)}
						<li>
							<a
								class="flex flex-wrap items-center gap-2 rounded-box bg-base-100 p-3 shadow-sm transition-colors hover:bg-base-200"
								href={runDetailPath(base, task.traceId, run.agentId)}
							>
								<span class="min-w-0">
									<span class="block text-sm font-medium">{run.bee || run.agentId}</span>
									<span class="block font-mono text-xs text-base-content/60">{run.agentId}</span>
									{#if taskRunMeta(run) !== ''}
										<span class="block text-xs text-base-content/50">{taskRunMeta(run)}</span>
									{/if}
								</span>
								<span class="ml-auto">
									<StatusBadge status={run.runStatus ?? ''} label={runStateLabel(run.runStatus ?? '')} />
								</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</Section>

		<!-- Approve and reject live here rather than only on the review queue: this is
		     where a task is read, and a gated task that could not be answered from the
		     page it is read on would be a step backwards from the legacy console. The
		     PR fields are collapsed under Approve, and Reject stays one box. -->
		{#if reviewable}
			<Section id="task-review" title="Review" note="this task is waiting on you">
				<ReviewActions {task} onsettled={settled} />
			</Section>
		{/if}
	{/if}
</div>
