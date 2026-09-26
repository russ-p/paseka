<script lang="ts">
	import Drawer from '$lib/components/Drawer.svelte';
	import { listBees } from '$lib/api/client';
	import { createSessionStore } from '$lib/stores/session.svelte';
	import type { Bee, Session } from '$lib/api/types';

	let {
		open,
		onclose,
		onlaunched,
		store = createSessionStore()
	}: {
		open: boolean;
		onclose: () => void;
		/** Fired with the started session, so the caller can navigate to it. */
		onlaunched?: (session: Session) => void;
		/**
		 * The launch itself goes through a store, so the list behind this drawer is
		 * already showing the new session by the time the page navigates — one owner for
		 * "what sessions exist", rather than a fetch here and a refresh there.
		 */
		store?: ReturnType<typeof createSessionStore>;
	} = $props();

	let bees = $state<Bee[]>([]);
	let loading = $state(true);
	let submitting = $state(false);
	let error = $state('');

	let bee = $state('');
	let body = $state('');
	let traceId = $state('');
	let intent = $state('');
	let rawMode = $state(false);
	let rawPrompt = $state('');

	/**
	 * A raw prompt replaces the task entirely: the server templates the task through the
	 * bee's prompt for the intent, and an override means the operator is supplying what
	 * the agent is launched with. So the two fields are alternatives rather than two
	 * fields, and only one is asked for.
	 */
	const prompt = $derived(rawMode ? rawPrompt : body);
	const empty = $derived(prompt.trim() === '');
	const ready = $derived(!empty && bee !== '' && !submitting);
	const selectedBee = $derived(bees.find((entry) => entry.role === bee));

	/**
	 * The bee's own prompt templates are the intents it can be launched under, so the
	 * list comes from the colony rather than from a constant here — a bee that adds an
	 * intent needs no console change. The task create form reads the same endpoint the
	 * same way.
	 */
	const intents = $derived(
		selectedBee
			? selectedBee.defaultIntent && !selectedBee.intents.includes(selectedBee.defaultIntent)
				? [...selectedBee.intents, selectedBee.defaultIntent]
				: selectedBee.intents
			: []
	);

	$effect(() => {
		if (!open) return;
		body = '';
		rawPrompt = '';
		bee = '';
		traceId = '';
		intent = '';
		rawMode = false;
		error = '';
		void load();
	});

	async function load(): Promise<void> {
		loading = true;
		error = '';
		try {
			bees = await listBees();
			// One bee is the common case on a fresh colony, so it is preselected rather
			// than making the operator choose the only option.
			if (bees.length === 1) selectBee(bees[0].role);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			loading = false;
		}
	}

	/** Default the intent to the bee's own, so the common case is one click. */
	function selectBee(role: string): void {
		bee = role;
		intent = bees.find((entry) => entry.role === role)?.defaultIntent ?? '';
	}

	async function submit(): Promise<void> {
		if (!ready) return;
		submitting = true;
		error = '';
		try {
			const created = await store.startSession({
				bee,
				useRawPrompt: rawMode,
				...(rawMode ? { rawPrompt: rawPrompt.trim() } : { body: body.trim() }),
				// An empty optional field is left out rather than sent as `""`: the server
				// treats a present-but-blank trace id differently from an absent one, where
				// absent means "generate one".
				...(traceId.trim() === '' ? {} : { traceId: traceId.trim() }),
				...(intent === '' ? {} : { intent })
			});
			onlaunched?.(created);
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
	title="Launch session"
	description="Start a bee you can talk to directly. The session runs a real terminal, so it can be attached to."
	onclose={onclose}
>
	{#if error}
		<div class="alert alert-error mb-3" role="alert"><span>{error}</span></div>
	{/if}

	{#if loading}
		<div class="space-y-2" aria-busy="true">
			<span class="skeleton block h-3 w-full"></span>
			<span class="skeleton block h-3 w-2/3"></span>
		</div>
	{:else if bees.length === 0}
		<div class="alert alert-warning" role="alert">
			<span>
				No launchable bees. <code class="font-mono">GET /api/bees</code> lists the bees whose
				adapter supports interactive sessions, so a colony of script adapters has none. A session
				can still be started from the terminal with
				<code class="font-mono">paseka bee chat</code>.
			</span>
		</div>
	{:else}
		<div class="space-y-4">
			<div class="fieldset">
				<label class="fieldset-legend" for="session-launch-bee">Bee</label>
				<select
					id="session-launch-bee"
					class="select select-bordered w-full"
					bind:value={bee}
					onchange={() => selectBee(bee)}
				>
					<option value="" disabled>Select a bee</option>
					{#each bees as entry (entry.role)}
						<option value={entry.role}>{entry.role} — {entry.adapter}</option>
					{/each}
				</select>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="session-launch-intent">Intent</label>
				<select
					id="session-launch-intent"
					class="select select-bordered w-full"
					disabled={intents.length === 0}
					bind:value={intent}
				>
					<option value="">None</option>
					{#each intents as name (name)}
						<option value={name}>{name}</option>
					{/each}
				</select>
				<span class="fieldset-label">
					{intents.length === 0
						? 'The selected bee declares no intents.'
						: `Prompt templates ${selectedBee?.role ?? ''} declares.`}
				</span>
			</div>

			<label class="flex cursor-pointer items-center gap-2">
				<input type="checkbox" class="checkbox checkbox-sm" bind:checked={rawMode} />
				<span class="label">Advanced: write the prompt myself</span>
			</label>

			{#if rawMode}
				<div class="fieldset">
					<label class="fieldset-legend" for="session-launch-raw">Raw prompt</label>
					<textarea
						id="session-launch-raw"
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
					<label class="fieldset-legend" for="session-launch-body">Task</label>
					<textarea
						id="session-launch-body"
						class="textarea textarea-bordered min-h-32 w-full"
						placeholder="What should this bee do?"
						bind:value={body}
					></textarea>
					<span class="fieldset-label">
						{intent === ''
							? 'With no intent the text is the whole task.'
							: `Templated through ${selectedBee?.role ?? 'the bee'}'s ${intent} prompt.`}
					</span>
				</div>
			{/if}

			<div class="fieldset">
				<label class="fieldset-legend" for="session-launch-trace">Trail ID</label>
				<input
					id="session-launch-trace"
					type="text"
					class="input input-bordered w-full font-mono"
					placeholder="auto-generated"
					bind:value={traceId}
				/>
				<span class="fieldset-label">
					Leave empty for a new trail, or name a standing one to hold the session with the rest
					of its work.
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
				{submitting ? 'Launching…' : 'Launch session'}
			</button>
		</div>
	{/snippet}
</Drawer>
