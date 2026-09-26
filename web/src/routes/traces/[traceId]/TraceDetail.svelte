<script lang="ts">
	import { base } from '$app/paths';
	import ArtifactViewModal from '$lib/components/ArtifactViewModal.svelte';
	import DetailRow from '$lib/components/DetailRow.svelte';
	import EnergyMeter from '$lib/components/EnergyMeter.svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import Section from '$lib/components/Section.svelte';
	import SignalCard from '$lib/components/SignalCard.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { createTraceStore, type TraceStore } from '$lib/stores/trace.svelte';
	import { toastStore, type ToastStore } from '$lib/stores/toast.svelte';
	import { consolePath, runDetailPath, taskDetailPath, traceTimelinePath } from '$lib/navigation';
	import {
		artifactLabel,
		artifactMeta,
		artifactState,
		formatTimestamp,
		runMeta,
		runStateLabel,
		taskMeta,
		taskPrimaryLabel,
		traceBees,
		traceFlags,
		tracePrimaryLabel,
		traceState,
		traceStateLabel,
		usageRows
	} from '$lib/format';
	import type { MetaRow } from '$lib/format';
	import type { ArtifactView } from '$lib/api/types';

	let {
		traceId,
		store = createTraceStore(),
		toasts = toastStore
	}: { traceId: string; store?: TraceStore; toasts?: ToastStore } = $props();

	$effect(() => {
		void store.load(traceId);
		store.start();
		return () => store.stop();
	});

	let viewing = $state<ArtifactView | null>(null);

	const detail = $derived(store.detail);
	const tasks = $derived(detail?.tasks ?? []);
	const runs = $derived(detail?.runs ?? []);
	const events = $derived(detail?.recentEvents ?? []);
	const worktree = $derived(detail?.worktree ?? null);
	const pullRequest = $derived(detail?.pullRequest ?? null);
	const usage = $derived(usageRows(detail?.usage));
	const flags = $derived(detail ? traceFlags(detail) : []);
	/** The collapsed block is a preview; the timeline is the full feed. */
	const previewEvents = $derived(events.slice(0, 8));
	const hiddenEvents = $derived(Math.max(0, events.length - previewEvents.length));

	const headline: MetaRow[] = $derived(
		detail
			? [
					{
						label: 'Trace',
						value: detail.traceId,
						mono: true,
						copy: true,
						// The copy button takes width, so the id truncates first; keep it readable.
						hint: [detail.traceId]
					},
					{ label: 'Last activity', value: formatTimestamp(detail.lastActivityAt) },
					{ label: 'Runs', value: String(detail.runCount) },
					{ label: 'Tasks', value: String(detail.taskCount) },
					{ label: 'Bees', value: traceBees(detail) },
					{ label: 'Flags', value: flags.length > 0 ? flags.join(', ') : '—' }
				]
			: []
	);

	const worktreeRows: MetaRow[] = $derived(
		worktree
			? [
					{ label: 'Path', value: worktree.path, mono: true, hint: [worktree.path] },
					{ label: 'Branch', value: worktree.branch || '—', mono: Boolean(worktree.branch) },
					{ label: 'Base SHA', value: worktree.baseSha || '—', mono: true },
					{ label: 'Created', value: formatTimestamp(worktree.createdAt) }
				]
			: []
	);

	const pullRequestRows: MetaRow[] = $derived(
		pullRequest?.url
			? [
					{
						label: 'Pull request',
						value: pullRequest.url,
						href: pullRequest.url,
						hint: [pullRequest.url, pullRequest.head ?? 'no head branch']
					}
				]
			: []
	);

	async function topUp(amount: number): Promise<void> {
		const ok = await store.topUp(amount);
		toasts.push(
			ok ? 'success' : 'error',
			ok ? `Honey reserve topped up by +${amount}` : (store.energyError || 'Top-up failed')
		);
	}
</script>

<svelte:head>
	<title>{detail ? tracePrimaryLabel(detail) : 'Trace'} · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="space-y-3">
		<a class="link text-sm text-base-content/60" href={consolePath(base, '/traces')}>← Traces</a>
		<div class="flex flex-wrap items-start justify-between gap-3">
			<div class="min-w-0 space-y-2">
				{#if store.showSkeletons}
					<span class="skeleton block h-8 w-2/3"></span>
				{:else}
					<h1 class="text-3xl font-bold" data-trace-heading={traceId}>
						{detail ? tracePrimaryLabel(detail) : traceId}
					</h1>
				{/if}
				{#if detail}
					<div class="flex flex-wrap items-center gap-2">
						{#if detail.standing}
							<StatusBadge status="standing" label="standing" />
						{/if}
						{#if detail.hasActive || detail.hasFailures}
							<StatusBadge status={traceState(detail)} label={traceState(detail)} />
						{/if}
					</div>
				{/if}
			</div>
			{#if detail}
				<a class="btn btn-sm" href={traceTimelinePath(base, traceId)}>Open timeline</a>
			{/if}
		</div>
		{#if detail?.summary}
			<p class="max-w-3xl text-base-content/70">{detail.summary}</p>
		{/if}
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-full"></span>
			<span class="skeleton block h-3 w-2/3"></span>
			<span class="skeleton block h-24 w-full"></span>
		</div>
	{:else if detail}
		<!-- Identity and the honey reserve answer "what is this trail" together, so they share a row on desktop. -->
		<div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
			<Section id="trace-trail" title="Trail" class="lg:col-span-2">
				<MetaList rows={headline} label="Trail" />
			</Section>

			<Section id="trace-honey" title="Honey reserve">
				<EnergyMeter energy={detail} pending={store.topUpPending} error={store.energyError} ontopup={topUp} />
			</Section>
		</div>

		<Section
			id="trace-artifacts"
			title="Trail artifacts"
			note={store.artifacts.length > 0 ? `${store.artifacts.length} in the comb` : undefined}
		>
			{#if store.artifactsError}
				<p class="text-sm text-error" role="alert">{store.artifactsError}</p>
			{:else if store.artifacts.length === 0}
				<p class="text-sm text-base-content/60">No trail artifacts in the comb.</p>
			{:else}
				<ul class="space-y-2">
					{#each store.artifacts as artifact (artifact.ref)}
						<DetailRow
							title={artifactLabel(artifact)}
							meta={artifactMeta(artifact)}
							dataKey={artifact.ref}
						>
							{#snippet side()}
								<StatusBadge status={artifactState(artifact)} label={artifactState(artifact)} />
								<button
									type="button"
									class="btn btn-ghost btn-xs"
									onclick={() => (viewing = artifact)}
								>
									View
								</button>
							{/snippet}
						</DetailRow>
					{/each}
				</ul>
			{/if}
		</Section>

		<Section id="trace-tasks" title="Tasks" note={tasks.length > 0 ? String(tasks.length) : undefined}>
			{#if tasks.length === 0}
				<p class="text-sm text-base-content/60">No tasks in this trail.</p>
			{:else}
				<ul class="space-y-2">
					{#each tasks as task (task.taskId)}
						<DetailRow
							title={taskPrimaryLabel(task)}
							meta={taskMeta(task)}
							href={taskDetailPath(base, detail.traceId, task.taskId)}
							dataKey={task.taskId}
						>
							{#snippet side()}
								<StatusBadge status={task.status} label={task.status} />
							{/snippet}
						</DetailRow>
					{/each}
				</ul>
			{/if}
		</Section>

		<Section id="trace-runs" title="Runs" note={runs.length > 0 ? String(runs.length) : undefined}>
			{#if runs.length === 0}
				<p class="text-sm text-base-content/60">No runs in this trail.</p>
			{:else}
				<ul class="space-y-2">
					{#each runs as run (run.agentId)}
						<DetailRow
							title={run.bee || run.agentId}
							meta={run.taskId ? `${run.agentId} · ${run.taskId}` : run.agentId}
							detail={runMeta(run)}
							href={runDetailPath(base, run.traceId, run.agentId)}
							dataKey={run.agentId}
						>
							{#snippet side()}
								<StatusBadge status={run.state} label={runStateLabel(run.state)} />
							{/snippet}
						</DetailRow>
					{/each}
				</ul>
			{/if}
		</Section>

		{#if worktree}
			<Section id="trace-worktree" title="Worktree" collapsible open={false}>
				<MetaList rows={worktreeRows} label="Worktree" />
				{#if pullRequestRows.length > 0}
					<MetaList rows={pullRequestRows} label="Pull request" />
				{/if}
			</Section>
		{/if}

		{#if usage.length > 0}
			<Section id="trace-usage" title="LLM usage" note={`${detail.usage?.runCountWithUsage ?? 0} runs reported`} collapsible>
				<MetaList rows={usage} label="LLM usage" />
			</Section>
		{/if}

		<Section
			id="trace-events"
			title="Recent events"
			note={events.length > 0 ? `last ${events.length}` : undefined}
			collapsible
			open={false}
		>
			{#if events.length === 0}
				<p class="text-sm text-base-content/60">No recent events.</p>
			{:else}
				<ul class="space-y-2">
					{#each previewEvents as event (event.id)}
						<SignalCard signal={event} showTrace={false} />
					{/each}
				</ul>
				{#if hiddenEvents > 0}
					<p class="mt-2 text-xs text-base-content/50">
						{hiddenEvents} older {hiddenEvents === 1 ? 'event' : 'events'} on the timeline.
					</p>
				{/if}
			{/if}
		</Section>
	{/if}
</div>

<ArtifactViewModal
	open={viewing !== null}
	{traceId}
	artifact={viewing}
	onclose={() => (viewing = null)}
/>
