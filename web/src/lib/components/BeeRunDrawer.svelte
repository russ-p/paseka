<script lang="ts">
	import { untrack } from 'svelte';
	import Drawer from '$lib/components/Drawer.svelte';
	import { listBees, runBee } from '$lib/api/client';
	import { beeIntents, beeNeedsTask } from '$lib/format';
	import type { Bee, RunBeeResult } from '$lib/api/types';

	let {
		open,
		bees,
		traceId: trail,
		onclose,
		onran
	}: {
		open: boolean;
		/**
		 * The roster, when the page already holds one. A headless run is offered for
		 * every bee, `script` ones included, so this can never be the launch picker
		 * (`GET /api/bees` without a scope) that the session drawer reads — those rows
		 * are the interactive bees, and a script bee would be missing from the one
		 * control that can start it.
		 *
		 * Omitted, the drawer reads the colony roster itself on open, which is what a
		 * route with no roster of its own (the trail page) wants. Passed, it costs no
		 * request and cannot disagree with the table the operator is reading above it.
		 */
		bees?: Bee[];
		/** Prefills the trail field — the trail a run started from inside a trail joins. */
		traceId?: string;
		onclose: () => void;
		/** Fired with the trail the run joins, so the caller can toast and refresh. */
		onran?: (result: RunBeeResult) => void;
	} = $props();

	let loaded = $state<Bee[] | undefined>(undefined);
	let loading = $state(false);
	let loadError = $state('');

	let role = $state('');
	let intent = $state('');
	let body = $state('');
	let traceId = $state('');
	let rawMode = $state(false);
	let rawPrompt = $state('');
	let submitting = $state(false);
	let error = $state('');

	/** The prop when the page passed a roster, otherwise the one read on open. */
	const roster = $derived(bees ?? loaded ?? []);

	/**
	 * A raw prompt replaces the task entirely — the server templates a task through
	 * the bee's prompt for the intent, and an override means the operator is
	 * supplying what the agent is launched with. So the two fields are alternatives,
	 * not two fields, and only one is asked for.
	 */
	const prompt = $derived(rawMode ? rawPrompt : body);
	const selected = $derived(roster.find((entry) => entry.role === role) ?? null);
	/**
	 * A script bee runs its own command, so an empty task is a run rather than a
	 * refusal — the same rule the server applies, and the reason a `script` bee is
	 * the one choice whose form can be submitted blank.
	 */
	const needsTask = $derived(beeNeedsTask(selected));
	const ready = $derived(!submitting && role !== '' && (prompt.trim() !== '' || !needsTask));

	/**
	 * The bee's own prompt templates are the intents it can be launched under, so the
	 * list comes from the colony rather than from a constant here — a bee that adds an
	 * intent needs no console change. `beeIntents` also folds in a default the
	 * discovery did not report, and copes with a bee that declares no intents at all:
	 * the server sends `null` there, not `[]`.
	 */
	const intents = $derived(beeIntents(selected));

	/**
	 * Opening is the only thing that resets the form, and it reads the roster and the
	 * trail untracked: a page that re-reads its own bees while the drawer is open would
	 * otherwise wipe a half-written task, and neither value can change under a form
	 * the operator is looking at.
	 */
	$effect(() => {
		if (!open) return;
		const fromPage = untrack(() => bees !== undefined);
		const known = untrack(() => bees ?? loaded ?? []);
		role = known.length === 1 ? (known[0]?.role ?? '') : '';
		intent = '';
		body = '';
		traceId = untrack(() => trail ?? '');
		rawMode = false;
		rawPrompt = '';
		error = '';
		loadError = '';
		if (!fromPage) void load();
	});

	async function load(): Promise<void> {
		loading = true;
		loadError = '';
		try {
			// `colony`, never the default scope: the picker that hides non-interactive
			// bees is the one a headless run must not be built on.
			loaded = await listBees('colony');
			if (loaded.length === 1) role = loaded[0]?.role ?? '';
		} catch (cause) {
			loadError = cause instanceof Error ? cause.message : String(cause);
		} finally {
			loading = false;
		}
	}

	/** Default the intent to the bee's own, so the common case is one field. */
	function selectBee(next: string): void {
		role = next;
		intent = roster.find((entry) => entry.role === next)?.defaultIntent ?? '';
	}

	async function submit(): Promise<void> {
		if (!ready || !selected) return;
		submitting = true;
		error = '';
		try {
			const result = await runBee(selected.role, {
				...(rawMode ? { inlinePrompt: rawPrompt.trim() } : { body: body.trim() }),
				// An empty optional field is left out rather than sent as `""`: the
				// server treats a present-but-blank trace id differently from an absent
				// one, where absent means "generate one".
				...(traceId.trim() === '' ? {} : { traceId: traceId.trim() }),
				...(intent === '' ? {} : { intent })
			});
			onran?.(result);
			onclose();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			submitting = false;
		}
	}
</script>

<Drawer
	{open}
	title="Run bee"
	description="One headless run, dispatched by the console itself — the hive runtime does not have to be up. Watch it under Runs."
	onclose={onclose}
>
	{#if error}
		<div class="alert alert-error mb-3" role="alert"><span>{error}</span></div>
	{/if}

	{#if loadError}
		<!-- The roster is the form, so a failed read is the form's problem to report
		     rather than an empty picker that reads as a colony with no bees. -->
		<div class="alert alert-error mb-3" role="alert"><span>{loadError}</span></div>
	{/if}

	{#if loading}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-3 w-full"></span>
			<span class="skeleton block h-3 w-2/3"></span>
		</div>
	{:else if roster.length === 0 && loadError === ''}
		<div class="alert alert-warning" role="alert">
			<span>
				This colony has no bees. A bee is registered under
				<code class="font-mono">.paseka/bees</code>, and a run can still be started from the
				terminal with <code class="font-mono">paseka bee run</code>.
			</span>
		</div>
	{:else}
		<div class="space-y-4">
			<div class="fieldset">
				<label class="fieldset-legend" for="bee-run-bee">Bee</label>
				<select
					id="bee-run-bee"
					class="select select-bordered w-full"
					bind:value={role}
					onchange={() => selectBee(role)}
				>
					<option value="" disabled>Select a bee</option>
					{#each roster as entry (entry.role)}
						<option value={entry.role}>{entry.role} — {entry.adapter}</option>
					{/each}
				</select>
				<span class="fieldset-label">
					Every bee in the roster, including the ones no session can start.
				</span>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="bee-run-intent">Intent</label>
				<select
					id="bee-run-intent"
					class="select select-bordered w-full"
					disabled={intents.length === 0 || !selected}
					bind:value={intent}
				>
					<option value="">None</option>
					{#each intents as name (name)}
						<option value={name}>{name}</option>
					{/each}
				</select>
				<span class="fieldset-label">
					{intents.length === 0
						? `${selected?.role ?? 'The selected bee'} declares no intents.`
						: `Prompt templates ${selected?.role ?? ''} declares.`}
				</span>
			</div>

			<label class="flex cursor-pointer items-center gap-2">
				<input type="checkbox" class="checkbox checkbox-sm" bind:checked={rawMode} />
				<span class="label">Advanced: write the prompt myself</span>
			</label>

			{#if rawMode}
				<div class="fieldset">
					<label class="fieldset-legend" for="bee-run-raw">Raw prompt</label>
					<textarea
						id="bee-run-raw"
						class="textarea textarea-bordered min-h-40 w-full font-mono text-xs"
						placeholder="Exactly what the agent is launched with"
						bind:value={rawPrompt}
					></textarea>
					<span class="fieldset-label">
						Sent verbatim. The task and the intent are not applied on top of it.
					</span>
				</div>
			{:else}
				<div class="fieldset">
					<label class="fieldset-legend" for="bee-run-body">Task</label>
					<textarea
						id="bee-run-body"
						class="textarea textarea-bordered min-h-32 w-full"
						placeholder="What should this bee do?"
						bind:value={body}
					></textarea>
					<span class="fieldset-label">
						{#if selected && !needsTask}
							{selected.role} is a script bee: it runs the command in its YAML, and a task
							here only becomes the prompt it is handed.
						{:else if intent === ''}
							With no intent the text is the whole task.
						{:else}
							Templated through {selected?.role ?? 'the bee'}'s {intent} prompt.
						{/if}
					</span>
				</div>
			{/if}

			<div class="fieldset">
				<label class="fieldset-legend" for="bee-run-trace">Trail ID</label>
				<input
					id="bee-run-trace"
					type="text"
					class="input input-bordered w-full font-mono"
					placeholder="auto-generated"
					bind:value={traceId}
				/>
				<span class="fieldset-label">
					{#if trail}
						Prefilled with the trail you opened this from, so the run joins it rather than
						opening a second one. Clear it for a new trail.
					{:else}
						Leave empty for a new trail, or name a standing one to hold this run with the rest
						of its work.
					{/if}
				</span>
			</div>
		</div>
	{/if}

	{#snippet footer()}
		<div class="flex items-center gap-2">
			<button type="button" class="btn btn-ghost btn-sm" onclick={onclose}>Cancel</button>
			<button
				type="button"
				class="btn btn-primary btn-sm"
				disabled={!ready}
				onclick={() => void submit()}
			>
				{submitting ? 'Starting…' : 'Run'}
			</button>
		</div>
	{/snippet}
</Drawer>