<script lang="ts">
	import { untrack } from 'svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import StatTile from '$lib/components/StatTile.svelte';
	import { formatTimestamp, formatTokenCount } from '$lib/format';
	import {
		createUsageStore,
		usageCustomRange,
		inclusiveEnd,
		type UsagePreset,
		type UsageStore
	} from '$lib/stores/usage.svelte';
	import type { UsageDay } from '$lib/api/types';

	let { store = createUsageStore() }: { store?: UsageStore } = $props();

	$effect(() => {
		// The store's read touches the range state it sends; untracked, so a range
		// change does not tear the mount effect down and start a second read.
		untrack(() => store.start());
		return () => store.stop();
	});

	let from = $state('');
	let to = $state('');
	let customError = $state('');
	const presets: UsagePreset[] = [7, 30, 90];

	// The inputs follow the range the report actually covers, so the control and
	// the table cannot name different dates. Written after the first payload
	// arrives rather than read from the store in the initializer, which would
	// only ever capture the store's first value.
	$effect(() => {
		const report = store.report;
		if (!report) return;
		from = report.since.slice(0, 10);
		to = inclusiveEnd(report.until.slice(0, 10));
	});

	function choose(preset: UsagePreset): void {
		customError = '';
		store.applyPreset(preset);
		from = store.since;
		to = inclusiveEnd(store.until);
	}

	function applyCustom(): void {
		customError = usageCustomRange(from, to)
			? ''
			: 'Choose a start date on or before the end date.';
		if (!customError) store.applyCustom(from, to);
	}

	function dayBadge(day: UsageDay): { status: string; label: string } {
		if (day.executions === 0) return { status: 'noexecutions', label: 'no executions' };
		if (day.unmeasured && day.incomplete) {
			return { status: 'unmeasured', label: 'unmeasured, incomplete' };
		}
		if (day.unmeasured) return { status: 'unmeasured', label: 'unmeasured' };
		if (day.incomplete) return { status: 'incomplete', label: 'incomplete' };
		return { status: 'reported', label: 'reported' };
	}

	const dayColumns: DataColumn<UsageDay>[] = [
		{ key: 'date', label: 'Day', text: (day) => day.date, mono: true },
		{ key: 'executions', label: 'Executions', text: (day) => String(day.executions), align: 'right' },
		{
			key: 'reported',
			label: 'Reported',
			text: (day) => String(day.reportedExecutions),
			align: 'right',
			secondary: true
		},
		{ key: 'trails', label: 'Trails', text: (day) => String(day.distinctTrails), align: 'right' },
		{
			key: 'input',
			label: 'Input',
			text: (day) => formatTokenCount(day.tokens.inputTokens),
			align: 'right'
		},
		{
			key: 'output',
			label: 'Output',
			text: (day) => formatTokenCount(day.tokens.outputTokens),
			align: 'right'
		},
		{
			key: 'reasoning',
			label: 'Reasoning',
			text: (day) => formatTokenCount(day.tokens.reasoningTokens),
			align: 'right',
			secondary: true
		},
		{
			key: 'cache-read',
			label: 'Cache read',
			text: (day) => formatTokenCount(day.tokens.cacheReadTokens),
			align: 'right',
			secondary: true
		},
		{
			key: 'cache-write',
			label: 'Cache write',
			text: (day) => formatTokenCount(day.tokens.cacheWriteTokens),
			align: 'right',
			secondary: true
		},
		{
			key: 'total',
			label: 'Total tokens',
			text: (day) => formatTokenCount(day.tokens.totalTokens),
			align: 'right'
		},
		{
			// Recorded provider cost, never computed from tokens. Blank when no
			// execution on the day recorded one; the count beside it says how many.
			key: 'cost',
			label: 'Recorded cost',
			text: (day) =>
				day.costReportedExecutions > 0
					? `$${day.recordedCostUsd.toFixed(2)}`
					: '—',
			align: 'right'
		},
		{
			key: 'cost-reported',
			label: 'Cost count',
			text: (day) => (day.costReportedExecutions > 0 ? String(day.costReportedExecutions) : '—'),
			align: 'right',
			secondary: true
		},
		{
			key: 'state',
			label: 'Measurement',
			// A day with executions but no reported usage must never read like an
			// empty day: this column names the difference.
			text: (day) => dayBadge(day).label,
			badge: dayBadge
		}
	];

	const period = $derived(store.report?.period);
	const components = $derived(
		period
			? [
					{ label: 'Input', value: period.tokens.inputTokens },
					{ label: 'Output', value: period.tokens.outputTokens },
					{ label: 'Reasoning', value: period.tokens.reasoningTokens },
					{ label: 'Cache read', value: period.tokens.cacheReadTokens },
					{ label: 'Cache write', value: period.tokens.cacheWriteTokens }
				]
			: []
	);
	const coverage = $derived(
		period ? `${period.reportedExecutions} of ${period.executions} reported` : '—'
	);
	const rangeLabel = $derived(
		store.report
			? `${store.report.since} to ${store.report.until} (${store.report.timezone})`
			: `${store.since} to ${store.until} (${store.timezone})`
	);
</script>

<svelte:head>
	<title>Usage · Queen Console</title>
</svelte:head>

<div class="space-y-6">
	<header class="space-y-2">
		<h1 class="text-3xl font-bold">Usage</h1>
		<p class="text-base-content/70">
			Token volume the colony recorded over retained runs, split into input, output, reasoning,
			cache read, and cache write. These are tokens, not money: a cache-heavy model can show
			millions of tokens at a small cost. Recorded provider cost appears in the daily table only
			where an adapter reported it, with the number of executions that did.
		</p>
	</header>

	<div class="flex flex-wrap items-end gap-3">
		<div class="join" role="group" aria-label="Usage range">
			{#each presets as days (days)}
				<button
					class="btn join-item btn-sm"
					class:btn-active={store.preset === days}
					aria-pressed={store.preset === days}
					onclick={() => choose(days)}
				>
					{days} days
				</button>
			{/each}
			<button
				class="btn join-item btn-sm"
				class:btn-active={store.preset === 'custom'}
				aria-pressed={store.preset === 'custom'}
				onclick={applyCustom}
			>
				Custom
			</button>
		</div>

		<div class="flex flex-wrap items-end gap-2">
			<label class="form-control">
				<span class="label-text text-xs">From</span>
				<input class="input input-sm" type="date" aria-label="From date" bind:value={from} />
			</label>
			<label class="form-control">
				<span class="label-text text-xs">To</span>
				<input class="input input-sm" type="date" aria-label="To date" bind:value={to} />
			</label>
			<button class="btn btn-sm" onclick={applyCustom}>Apply</button>
		</div>

		<button class="btn btn-sm btn-outline ml-auto" onclick={() => store.refresh()} disabled={store.loading}>
			Refresh
		</button>
	</div>

	{#if customError}
		<div class="alert alert-warning" role="alert"><span>{customError}</span></div>
	{/if}

	{#if store.lastError}
		<div class="alert alert-error" role="alert">
			<span>{store.lastError}</span>
		</div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<span class="skeleton block h-3 w-1/3"></span>
			<span class="skeleton block h-40 w-full"></span>
			<span class="skeleton block h-64 w-full"></span>
		</div>
	{:else if store.report && period}
		<p class="text-xs text-base-content/60">
			{rangeLabel} · observed {formatTimestamp(store.report.observedAt)} · start-time buckets, so a
			run crossing midnight stays on its start day
		</p>

		<section class="space-y-3" aria-labelledby="usage-headline">
			<h2 id="usage-headline" class="text-xl font-semibold">Token volume</h2>

			<div class="rounded-box border border-base-300 bg-base-100 p-4">
				<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
					{#each components as component (component.label)}
						<div>
							<p class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">
								{component.label}
							</p>
							<p class="text-2xl leading-tight font-bold">{formatTokenCount(component.value)}</p>
						</div>
					{/each}
					<div>
						<p class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">Total</p>
						<p class="text-2xl leading-tight font-bold">{formatTokenCount(period.tokens.totalTokens)}</p>
					</div>
				</div>
				<p class="mt-3 text-xs text-base-content/60">
					Total is the sum of these reported components. Reasoning is not folded into output, and
					cache kinds are not folded into input.
				</p>
			</div>

			<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
				<StatTile
					label="Executions"
					value={String(period.executions)}
					hint={[`${period.reportedExecutions} reported usage`, `${period.executions - period.reportedExecutions} did not`]}
				/>
				<StatTile label="Trails" value={String(period.distinctTrails)} />
				<StatTile
					label="Coverage"
					value={coverage}
					hint={[
						store.report.earliestRetainedAt
							? `Retained record begins ${formatTimestamp(store.report.earliestRetainedAt)}`
							: 'No retained execution record is dated'
					]}
				/>
			</div>
		</section>

		{#if store.report.warnings.length > 0}
			<div class="alert alert-warning" role="status">
				<ul class="list-disc space-y-1 pl-5">
					{#each store.report.warnings as warning, index (index)}
						<li>{warning.detail}</li>
					{/each}
				</ul>
			</div>
		{/if}

		<section class="space-y-3" aria-labelledby="usage-daily">
			<h2 id="usage-daily" class="text-xl font-semibold">Daily volume</h2>
			{#if period.executions === 0}
				<p class="text-base-content/70">
					No executions in this range. The days below are still listed, zero-filled, so two ranges
					share the same axis.
				</p>
			{/if}
			<DataTable
				label="Daily usage"
				columns={dayColumns}
				rows={store.report.days}
				rowKey={(day) => day.date}
				emptyMessage="No days in this range."
				filterLabel="Filter days"
				filterPlaceholder="day or measurement"
				pageSize={0}
			/>
			<p class="text-xs text-base-content/50">
				Recorded cost is the provider's own number, shown only where an adapter reported it, with
				the count of executions that did. It is never estimated from tokens.
			</p>
		</section>
	{/if}
</div>
