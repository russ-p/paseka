<script lang="ts">
	import { base } from '$app/paths';
	import { RefreshCw } from 'lucide-svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { DataColumn } from '$lib/components/DataTable.svelte';
	import MetaList from '$lib/components/MetaList.svelte';
	import Section from '$lib/components/Section.svelte';
	import StatTile from '$lib/components/StatTile.svelte';
	import type { AdapterCLI, SystemProcess } from '$lib/api/types';
	import {
		adapterCLIsNote,
		adapterPresenceBadge,
		cpuPendingHint,
		formatPercent,
		formatSize,
		liveBeePids,
		systemAvailableWord,
		systemCpuPending,
		systemDiskWord,
		systemIdentityRows,
		systemLoadWord,
		systemMemoryWord
	} from '$lib/format';
	import {
		createAdapterCLIsStore,
		type AdapterCLIsStore
	} from '$lib/stores/adapters.svelte';
	import { createSystemStore, type SystemStore } from '$lib/stores/system.svelte';
	import {
		consoleStatusStore,
		type ConsoleStatusStore
	} from '$lib/stores/console-status.svelte';

	let {
		store = createSystemStore(),
		status = consoleStatusStore,
		adapters = createAdapterCLIsStore()
	}: { store?: SystemStore; status?: ConsoleStatusStore; adapters?: AdapterCLIsStore } = $props();

	$effect(() => {
		store.start();
		return () => store.stop();
	});

	const system = $derived(store.system);
	const identityRows = $derived(systemIdentityRows(system));
	const processes = $derived(store.processes);
	/**
	 * The adapter pids come from the chrome stream the topbar already keeps, so
	 * marking a bee costs no extra request and cannot disagree with the plaque.
	 */
	const beePids = $derived(liveBeePids(status.agents?.items));

	const cpuWord = $derived(formatPercent(system?.cpuPercent));
	const memoryWord = $derived(systemMemoryWord(system));
	const availableWord = $derived(systemAvailableWord(system));
	const loadWord = $derived(systemLoadWord(system));
	const diskWord = $derived(systemDiskWord(system));
	const cpuPending = $derived(systemCpuPending(system));

	const processColumns: DataColumn<SystemProcess>[] = [
		{ key: 'pid', label: 'PID', text: (process) => String(process.pid), mono: true },
		{
			key: 'cpu',
			label: 'CPU',
			// Absent on a process the sampler could not read, so the cell goes quiet
			// rather than claiming a process is idle.
			text: (process) => formatPercent(process.cpuPercent) ?? '',
			align: 'right'
		},
		{
			key: 'rss',
			label: 'RSS',
			text: (process) => formatSize(process.rssBytes) ?? '',
			align: 'right',
			secondary: true
		},
		{ key: 'comm', label: 'Name', text: (process) => process.comm || '—', mono: true },
		{
			key: 'source',
			label: 'Source',
			// The legacy console marked a live adapter with a row class. A badge is the
			// contract instead: the tone table owns the colour and the row keeps its meaning.
			// The declared-but-null badge leaves an ordinary process with an empty cell.
			text: () => '',
			badge: (process) => (beePids.has(process.pid) ? { status: 'live', label: 'bee' } : null)
		},
		// The command line is what tells jest from the cursor agent, so it takes the
		// leftover width; the server truncates it at 200 runes for the same reason.
		{ key: 'cmd', label: 'Command', text: (process) => process.cmd || '—', mono: true, grow: true }
	];

	/**
	 * The Agent CLIs block's rows. Identity, state, then the two facts an operator
	 * checks a CLI by — and the path behind them, which is what makes two rows with
	 * the same name and version explainable. The path is `secondary` because it is
	 * the reference behind the identity, so it is the first thing to go on a phone
	 * and the last thing an operator misses; `found` keeps its cell empty for the
	 * three facts rather than dashing them, the way the Live bees badge does.
	 */
	const adapterColumns: DataColumn<AdapterCLI>[] = [
		{ key: 'name', label: 'Adapter', text: (adapter) => adapter.name, mono: true },
		{
			key: 'found',
			label: 'Found',
			text: (adapter) => (adapter.found ? 'found' : 'not found'),
			badge: (adapter) => adapterPresenceBadge(adapter)
		},
		{
			key: 'path',
			label: 'Path',
			text: (adapter) => adapter.path || '',
			mono: true,
			grow: true,
			secondary: true
		},
		{ key: 'version', label: 'Version', text: (adapter) => adapter.version || '', mono: true },
		// The reason a probe that found a binary still has no version, which is the
		// one row state the other columns cannot express.
		{ key: 'probe', label: 'Probe', text: (adapter) => adapter.error || '' }
	];

	const adapterNote = $derived(adapterCLIsNote(adapters.adapters, adapters.probed));
</script>

<svelte:head>
	<title>System · Queen Console Next</title>
</svelte:head>

<div class="space-y-6">
	<header class="space-y-1">
		<h1 class="text-3xl font-bold">System</h1>
		<p class="text-base-content/70">
			The OS as seen by the <span class="font-mono">paseka console</span> process. Observe-only:
			there are no kill, nice, or signal controls here, and a process name is a hint that work is
			happening, not a ledger of what the colony started.
		</p>
	</header>

	{#if store.lastError}
		<div class="alert alert-error" role="alert"><span>{store.lastError}</span></div>
	{/if}

	<!-- A partial snapshot arrives on a 200, so it is a warning about missing rows
	     rather than a broken page, and it must not replace what did arrive. -->
	{#if store.partialError}
		<div class="alert alert-warning" role="alert">
			<span>Partial snapshot: {store.partialError}</span>
		</div>
	{/if}

	{#if store.showSkeletons}
		<div class="space-y-3" aria-busy="true">
			<div class="grid grid-cols-2 gap-3 lg:grid-cols-5">
				<span class="skeleton block h-16 w-full"></span>
				<span class="skeleton block h-16 w-full"></span>
				<span class="skeleton block h-16 w-full"></span>
				<span class="skeleton block h-16 w-full"></span>
				<span class="skeleton block h-16 w-full"></span>
			</div>
			<span class="skeleton block h-40 w-full"></span>
		</div>
	{:else if system}
		<!-- A tile appears only when the server measured the value, so a degraded box
		     looks degraded instead of padded with dashes. CPU is the exception: its
		     first-sample gap is expected, so the tile stays and explains itself. -->
		<div class="grid grid-cols-2 gap-3 lg:grid-cols-5">
			<StatTile
				id="system-cpu"
				label="CPU"
				value={cpuWord ?? '—'}
				hint={cpuPending ? cpuPendingHint : undefined}
			/>
			{#if memoryWord}
				<StatTile id="system-memory" label="Memory" value={memoryWord} hint={[memoryWord]} />
			{/if}
			{#if availableWord}
				<StatTile
					id="system-available"
					label="Available"
					value={availableWord}
					hint={[`${availableWord} before the box starts swapping`]}
				/>
			{/if}
			{#if loadWord}
				<StatTile
					id="system-load"
					label="Load 1/5/15"
					value={loadWord}
					hint={['One, five, and fifteen minute averages', system.cpus
						? `A load above ${system.cpus} means more runnable work than cores`
						: 'CPU count unavailable']}
				/>
			{/if}
			{#if diskWord}
				<StatTile
					id="system-disk"
					label="Colony disk"
					value={diskWord}
					hint={['Used and total on the volume holding the colony root']}
				/>
			{/if}
		</div>

		<Section id="system-identity" title="Identity">
			<MetaList rows={identityRows} label="Host identity" columns={2} />
			<p class="mt-3 text-xs text-base-content/50">
				In a container this is the container's PID namespace, and the process list is the
				interesting part of it. Memory and load still come from the host's
				<code class="font-mono">/proc</code> unless cgroups hide it, so treat them as the machine
				rather than a container limit.
			</p>
		</Section>

		<!-- 25 rows of process table is the biggest thing on the page, and the metrics
		     above are what an operator opens System for. It starts folded, and the summary
		     line says how much is in there. -->
		<Section
			id="system-processes"
			title="Processes"
			note={processes.length > 0 ? `${processes.length} busiest` : 'unavailable'}
			collapsible
			open={false}
		>
			<DataTable
				label="Processes"
				columns={processColumns}
				rows={processes}
				rowKey={(process) => String(process.pid)}
				emptyMessage="No process list — unavailable on this OS, or /proc could not be read."
				filterLabel="Filter processes"
				filterPlaceholder="name, command, pid"
				pageSize={25}
			/>
			<!-- The two CPU numbers on this page have different denominators, and a row
			     reading 172% next to a tile reading 15% otherwise looks like a bug. -->
			<p class="text-xs text-base-content/50">
				CPU per process is measured against one core, so a busy row reads above 100% on a
				multi-core box; the CPU tile above is the whole machine. Kernel threads are omitted, and
				the server sends the 25 busiest processes rather than every pid on the box.
			</p>
		</Section>

		<!-- Which agent CLIs this box can actually run. It stays folded and unfetched: the probe
		     execs four external binaries, the server caches what it found, and an operator who never
		     opens this block should not pay for it. Opening it is the request — once, since the
		     answer is cached server-side until Refresh. -->
		<Section
			id="system-agent-clis"
			title="Agent CLIs"
			note={adapterNote}
			collapsible
			open={false}
			ontoggle={(open) => open && void adapters.load()}
		>
			{#snippet actions()}
				<button
					id="system-agent-clis-refresh"
					type="button"
					class="btn btn-ghost btn-sm"
					disabled={adapters.loading}
					aria-busy={adapters.loading}
					onclick={() => void adapters.refresh()}
				>
					<RefreshCw class="h-4 w-4 {adapters.loading ? 'animate-spin' : ''}" strokeWidth={2.5} />
					Refresh
				</button>
			{/snippet}

			{#if adapters.lastError}
				<div class="alert alert-error mb-3" role="alert"><span>{adapters.lastError}</span></div>
			{/if}

			<DataTable
				label="Agent CLIs"
				columns={adapterColumns}
				rows={adapters.adapters}
				rowKey={(adapter) => adapter.name}
				emptyMessage="No agent CLI reported — the server probed none."
				filterLabel="Filter agent CLIs"
				filterPlaceholder="name, binary, version"
				pageSize={0}
				// The processes table above keeps the default `?q=&page=`, so this one
				// takes a namespace rather than letting two tables on one route fight
				// over the same params — and rather than changing the links to
				// `/next/system` an operator has already shared.
				stateKey="agent-clis"
				loading={adapters.loading && !adapters.probed}
			/>
			<p class="mt-3 text-xs text-base-content/50">
				Which agent CLIs this box can launch, probed against the binary the colony resolved for
				each adapter — so a <span class="font-mono">binary:</span> in
				<span class="font-mono">~/.config/paseka/&lt;slug&gt;/adapters/&lt;name&gt;.yaml</span>
				is what the row reports, not the loader's default. A row reading
				<span class="font-mono">not found</span> is the adapter's state, not a fault: the colony
				can still declare bees for it, and the console says nothing about credentials — the
				version probe runs without them.
			</p>
			<p class="mt-1 text-xs text-base-content/50">
				<span class="font-mono">script</span> is absent on purpose: its binary is whatever each
				bee's <span class="font-mono">command:</span> says, so there is no single entry here to
				look for. Refresh re-runs the probe; the server caches the answer until you do.
			</p>
		</Section>
	{/if}
</div>
