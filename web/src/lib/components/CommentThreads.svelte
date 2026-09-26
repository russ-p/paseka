<script lang="ts">
	import { MessageSquarePlus, Trash2 } from 'lucide-svelte';
	import { rejectTask } from '$lib/api/client';
	import type { ReviewComment, ReviewQueueItem, TaskDetail } from '$lib/api/types';
	import type { DiffAnchor } from '$lib/diff';

	let {
		task,
		headSha = '',
		onsettled
	}: {
		task: TaskDetail | ReviewQueueItem;
		/** Pins the packet to the commit it was read against. */
		headSha?: string;
		onsettled?: (outcome: { tone: 'success' | 'warning'; message: string }) => void;
	} = $props();

	/** A draft, keyed by the range it covers: one note per range, not per click. */
	interface Draft {
		path: string;
		side: 'old' | 'new';
		startLine: number;
		endLine: number;
		snippet: string;
		body: string;
	}

	let drafts = $state<Draft[]>([]);
	/**
	 * A pending note is a *range*, so both ends are kept: the first click sets both,
	 * and a second click in the same file and on the same side moves only the far
	 * end. That is how a note about a block of lines is written without dragging.
	 */
	let pending = $state<{ start: DiffAnchor; end: DiffAnchor } | null>(null);
	let editing = $state('');
	let noteBody = $state('');
	let summary = $state('');
	let submitting = $state(false);
	let error = $state('');

	function draftId(draft: Pick<Draft, 'path' | 'side' | 'startLine' | 'endLine'>): string {
		return `${draft.path}:${draft.side}:${draft.startLine}:${draft.endLine || draft.startLine}`;
	}

	/**
	 * A moved head voids the packet. The line numbers a note is anchored to are
	 * meaningless against a different commit, and sending them anyway would attach
	 * the bee's rework to the wrong lines — so drafts and the summary are dropped
	 * rather than silently re-aimed.
	 */
	let pinnedSha = $state('');
	$effect(() => {
		const next = headSha;
		if (pinnedSha !== '' && next !== '' && pinnedSha !== next) {
			drafts = [];
			summary = '';
			pending = null;
			editing = '';
		}
		if (next !== '') pinnedSha = next;
	});

	/**
	 * Called by the diff body when a line is clicked. Exposed on the instance so the
	 * page can wire `DiffViewer`'s callback to it without either component owning
	 * the other's state.
	 */
	export function anchorOn(clicked: DiffAnchor): void {
		if (
			pending &&
			pending.start.path === clicked.path &&
			pending.start.side === clicked.side
		) {
			pending = { start: pending.start, end: clicked };
		} else {
			pending = { start: clicked, end: clicked };
		}
		noteBody = '';
		editing = '';
	}

	const rangeStart = $derived(
		pending ? Math.min(pending.start.line, pending.end.line) : 0
	);
	const rangeEnd = $derived(pending ? Math.max(pending.start.line, pending.end.line) : 0);
	/** The far end's text, because that is the line the second click named. */
	const rangeSnippet = $derived(pending ? (pending.end.snippet || pending.start.snippet) : '');

	const canSave = $derived(pending !== null && noteBody.trim() !== '');
	const canSubmit = $derived(
		!submitting && (drafts.length > 0 || summary.trim() !== '')
	);
	/** A rework task already in flight means the bee is busy; a second one would collide. */
	const reworkInFlight = $derived(task.reworkTaskId !== undefined && task.reworkTaskId !== '');
	const blocked = $derived(reworkInFlight || task.canRequestChanges === false);

	function save(): void {
		if (!canSave || pending === null) return;
		const draft: Draft = {
			path: pending.start.path,
			side: pending.start.side,
			startLine: rangeStart,
			endLine: rangeEnd === rangeStart ? rangeStart : rangeEnd,
			snippet: rangeSnippet,
			body: noteBody.trim()
		};
		const id = draftId(draft);
		const at = drafts.findIndex((entry) => draftId(entry) === id);
		if (at >= 0) drafts[at] = draft;
		else if (editing !== '') drafts = drafts.map((entry) => (draftId(entry) === editing ? draft : entry));
		else drafts = [...drafts, draft];
		pending = null;
		editing = '';
		noteBody = '';
	}

	function edit(draft: Draft): void {
		const anchor: DiffAnchor = {
			path: draft.path,
			side: draft.side,
			line: draft.startLine,
			snippet: draft.snippet
		};
		pending = { start: anchor, end: anchor };
		noteBody = draft.body;
		editing = draftId(draft);
	}

	function remove(draft: Draft): void {
		drafts = drafts.filter((entry) => draftId(entry) !== draftId(draft));
		if (editing === draftId(draft)) {
			editing = '';
			pending = null;
			noteBody = '';
		}
	}

	/**
	 * The wire shape, and the reason for two of its rules: a single-line note omits
	 * `endLine` and an empty snippet is omitted, because the bee's rework prompt
	 * quotes the range and a zero-width range reads as a mistake.
	 */
	const payload = $derived<ReviewComment[]>(
		drafts.map((draft) => ({
			path: draft.path,
			side: draft.side,
			startLine: draft.startLine,
			...(draft.endLine !== draft.startLine ? { endLine: draft.endLine } : {}),
			...(draft.snippet === '' ? {} : { snippet: draft.snippet }),
			body: draft.body
		}))
	);

	async function submit(): Promise<void> {
		if (!canSubmit) return;
		submitting = true;
		error = '';
		try {
			const result = await rejectTask(task.traceId, task.taskId, {
				feedback: summary.trim(),
				...(pinnedSha === '' ? {} : { headSha: pinnedSha }),
				comments: payload
			});
			onsettled?.({
				tone: 'warning',
				message: result.message ?? 'Review comments submitted.'
			});
			drafts = [];
			summary = '';
			pending = null;
			editing = '';
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			submitting = false;
		}
	}
</script>

<section class="space-y-3" aria-label="Review comments">
	<header class="flex items-center justify-between gap-2">
		<h3 class="text-xs font-semibold tracking-wide text-base-content/60 uppercase">Review comments</h3>
		<span class="text-xs text-base-content/50">{drafts.length} draft{drafts.length === 1 ? '' : 's'}</span>
	</header>

	<p class="text-xs text-base-content/60">
		Click a line in the diff to leave a note on it; click a second line in the same file to widen the
		range. Drafts stay in this browser until you send them.
	</p>

	{#if reworkInFlight}
		<p class="text-xs text-warning">
			Rework {task.reworkTaskId} is {task.reworkStatus ?? 'in flight'}. Request changes is
			disabled until it finishes.
		</p>
	{:else if task.canRequestChanges === false}
		<p class="text-xs text-warning">
			Request changes is unavailable: the proposal bee is not isolated, or a rework task is already in
			flight.
		</p>
	{/if}

	{#if error}
		<div class="alert alert-error" role="alert"><span>{error}</span></div>
	{/if}

	{#if pending}
		<div class="rounded-box border border-base-300 bg-base-200/40 p-3">
			<p class="font-mono text-xs">
				{pending.start.path} · {pending.start.side} L{rangeStart}{rangeEnd !== rangeStart
					? `–${rangeEnd}`
					: ''}
			</p>
			<label class="fieldset-legend mt-1" for="review-comment-body">Note</label>
			<textarea
				id="review-comment-body"
				class="textarea textarea-bordered min-h-20 w-full"
				placeholder="What should change, and why"
				bind:value={noteBody}
			></textarea>
			<div class="mt-2 flex items-center gap-2">
				<button
					type="button"
					class="btn btn-primary btn-sm"
					disabled={!canSave}
					onclick={save}
				>
					<MessageSquarePlus class="h-4 w-4" strokeWidth={2.5} />
					{editing === '' ? 'Add draft' : 'Update draft'}
				</button>
				<button
					type="button"
					class="btn btn-ghost btn-sm"
					onclick={() => {
						pending = null;
						editing = '';
						noteBody = '';
					}}
				>
					Cancel
				</button>
			</div>
		</div>
	{/if}

	{#if drafts.length > 0}
		<ul class="space-y-2">
			{#each drafts as draft (draftId(draft))}
				{@const id = draftId(draft)}
				<li class="rounded-box border border-base-300 p-2">
					<p class="font-mono text-xs">
						{draft.path} · {draft.side} L{draft.startLine}{draft.endLine !== draft.startLine
							? `–${draft.endLine}`
							: ''}
					</p>
					<p class="mt-1 text-sm">{draft.body}</p>
					<div class="mt-1 flex items-center gap-2">
						<button type="button" class="btn btn-ghost btn-xs" onclick={() => edit(draft)}>
							Edit
						</button>
						<button
							type="button"
							class="btn btn-ghost btn-xs text-error"
							aria-label={`Delete comment on ${draft.path} line ${draft.startLine}`}
							onclick={() => remove(draft)}
						>
							<Trash2 class="h-3.5 w-3.5" strokeWidth={2.5} />
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	<form
		class="space-y-2"
		onsubmit={(event) => {
			event.preventDefault();
			void submit();
		}}
	>
		<label class="fieldset-legend" for="review-comments-summary">Overall</label>
		<textarea
			id="review-comments-summary"
			class="textarea textarea-bordered min-h-16 w-full"
			placeholder="The theme of this review, for the rework prompt"
			bind:value={summary}
		></textarea>
		<button
			type="submit"
			class="btn btn-warning btn-sm"
			disabled={!canSubmit || blocked}
		>
			{submitting ? 'Sending…' : 'Request changes'}
		</button>
	</form>
</section>
