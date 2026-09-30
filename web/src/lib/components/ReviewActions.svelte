<script lang="ts">
	import { approveTask, rejectTask } from '$lib/api/client';
	import type { PullRequest, ReviewQueueItem, TaskDetail } from '$lib/api/types';

	let {
		task,
		onsettled
	}: {
		task: TaskDetail | ReviewQueueItem;
		/** Fired after an approve or reject lands, so the caller can refresh and toast. */
		onsettled?: (outcome: { tone: 'success' | 'warning'; message: string }) => void;
	} = $props();

	/**
	 * Both forms are collapsed until asked. The common action is approve-or-read, and
	 * a form of six fields above the fold competes with reading the diff.
	 */
	let open = $state<'approve' | 'reject' | null>(null);
	let summary = $state('');
	let mergeMessage = $state('');
	let prTitle = $state('');
	let prBody = $state('');
	let draft = $state(false);
	let runHooks = $state(false);
	let feedback = $state('');
	let submitting = $state<'approve' | 'reject' | null>(null);
	let error = $state('');

	/**
	 * `colony.DeliveryPullRequest` is `pull_request`, and `local_merge` is the
	 * other value. An earlier version of the task page compared against `pr`, which
	 * no delivery is ever equal to, so the pull-request fields silently never
	 * appeared on a colony that publishes them.
	 */
	const isPR = $derived(task.delivery === 'pull_request');
	const isFinal = $derived(task.isFinal);
	/** A non-final review merges nothing, so it has no commit message to set. */
	const showMergeFields = $derived(isFinal && !isPR);
	const showPrFields = $derived(isFinal && isPR);
	/** The server's own copy for the PR, pre-filled into an empty field only. */
	const serverPrTitle = $derived(task.prTitle ?? '');
	const serverPrBody = $derived(task.prBody ?? '');
	const prUrl = $derived(task.pullRequest?.url);

	$effect(() => {
		// Pre-fill from the server's copy, but never over what the operator typed:
		// re-rendering a form must not discard a half-written title.
		if (showPrFields && prTitle === '' && serverPrTitle !== '') prTitle = serverPrTitle;
		if (showPrFields && prBody === '' && serverPrBody !== '') prBody = serverPrBody;
	});

	const approveLabel = $derived(
		showPrFields ? (prUrl ? 'Update PR' : 'Open PR') : 'Approve'
	);
	const approveReady = $derived(
		submitting === null && summary.trim() !== '' && (showPrFields === false || prTitle.trim() !== '')
	);
	const rejectReady = $derived(submitting === null && feedback.trim() !== '');

	/** What approving will actually do, said before the operator commits to it. */
	const approveNote = $derived(
		showPrFields
			? prUrl
				? 'Pushes the worktree branch and updates its pull request.'
				: 'Pushes the worktree branch and opens a pull request for it.'
			: isFinal
				? 'Merges this trail’s worktree branch into the default branch.'
				: 'Records your sign-off. The bee’s work counts as accepted.'
	);

	async function approve(): Promise<void> {
		if (!approveReady) return;
		submitting = 'approve';
		error = '';
		try {
			const result = await approveTask(task.traceId, task.taskId, {
				summary: summary.trim(),
				...(mergeMessage.trim() === '' ? {} : { mergeMessage: mergeMessage.trim() }),
				...(showPrFields ? { prTitle: prTitle.trim(), prBody: prBody.trim(), draft, runHooks } : {})
			});
			onsettled?.({
				tone: 'success',
				message:
					result.message ??
					(result.prUrl ? `${approveLabel}: ${result.prUrl}` : `Approved ${task.taskId}`)
			});
			reset();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			submitting = null;
		}
	}

	async function reject(): Promise<void> {
		if (!rejectReady) return;
		submitting = 'reject';
		error = '';
		try {
			const result = await rejectTask(task.traceId, task.taskId, { feedback: feedback.trim() });
			onsettled?.({
				tone: 'warning',
				message:
					result.message ??
					(result.reworkTaskId
						? `Rework task ${result.reworkTaskId} created`
						: `Rejected ${task.taskId}`)
			});
			reset();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : String(cause);
		} finally {
			submitting = null;
		}
	}

	function reset(): void {
		open = null;
		summary = '';
		mergeMessage = '';
		prTitle = '';
		prBody = '';
		draft = false;
		runHooks = false;
		feedback = '';
	}
</script>

<!-- An ellipsis says the verb opens something, which also keeps the toggle and the
     submit from being two buttons with the same accessible name. -->
<div class="flex flex-wrap items-center gap-2">
	<button
		id="task-approve"
		type="button"
		class="btn btn-success btn-sm"
		aria-expanded={open === 'approve'}
		onclick={() => {
			open = open === 'approve' ? null : 'approve';
			error = '';
		}}
	>
		{approveLabel}…
	</button>
	<button
		id="task-reject"
		type="button"
		class="btn btn-error btn-sm btn-outline"
		aria-expanded={open === 'reject'}
		onclick={() => {
			open = open === 'reject' ? null : 'reject';
			error = '';
		}}
	>
		Reject…
	</button>
</div>

{#if open === 'approve'}
	<form
		id="task-approve-form"
		class="mt-3 space-y-3 rounded-box border border-base-300 bg-base-200/40 p-3"
		onsubmit={(event) => {
			event.preventDefault();
			void approve();
		}}
	>
		{#if error}
			<div class="alert alert-error" role="alert"><span>{error}</span></div>
		{/if}

		<div class="fieldset">
			<label class="fieldset-legend" for="task-approve-summary">Approval summary</label>
			<textarea
				id="task-approve-summary"
				class="textarea textarea-bordered min-h-20 w-full"
				placeholder="What you checked, and what you are accepting"
				bind:value={summary}
			></textarea>
			<span class="fieldset-label">Required. Recorded against the task.</span>
		</div>

		{#if showMergeFields}
			<div class="fieldset">
				<label class="fieldset-legend" for="task-approve-merge">Commit message</label>
				<input
					id="task-approve-merge"
					type="text"
					class="input input-bordered w-full font-mono"
					placeholder="optional — the trail summary is used when empty"
					bind:value={mergeMessage}
				/>
			</div>
		{/if}

		{#if showPrFields}
			<div class="fieldset">
				<label class="fieldset-legend" for="task-pr-title">PR title</label>
				<input
					id="task-pr-title"
					type="text"
					class="input input-bordered w-full"
					placeholder="What this change does"
					bind:value={prTitle}
				/>
			</div>

			<div class="fieldset">
				<label class="fieldset-legend" for="task-pr-body">PR body</label>
				<textarea
					id="task-pr-body"
					class="textarea textarea-bordered min-h-24 w-full"
					placeholder="Why, and what a reviewer should look at"
					bind:value={prBody}
				></textarea>
			</div>

			<div class="flex flex-wrap items-center gap-4">
				<label class="flex cursor-pointer items-center gap-2">
					<input type="checkbox" class="checkbox checkbox-sm" bind:checked={draft} />
					<span class="label">Open as draft</span>
				</label>
				<label class="flex cursor-pointer items-center gap-2">
					<input type="checkbox" class="checkbox checkbox-sm" bind:checked={runHooks} />
					<span class="label">Run git hooks on push</span>
				</label>
			</div>
		{/if}

		<div class="flex flex-wrap items-center gap-2">
			<button type="submit" class="btn btn-success btn-sm" disabled={!approveReady}>
				{submitting === 'approve' ? `${approveLabel}…` : approveLabel}
			</button>
			<button type="button" class="btn btn-ghost btn-sm" onclick={reset}>Cancel</button>
			<span class="text-xs text-base-content/50">{approveNote}</span>
		</div>
	</form>
{:else if open === 'reject'}
	<!-- Reject stays minimal: a line of feedback the bee gets as rework input.
	     Everything else the operator might want to say belongs in the PR or the
	     commit, not in a second copy of the task body. Request changes, which does
	     start a rework task and carries an annotated packet, lives with the diff. -->
	<form
		id="task-reject-form"
		class="mt-3 space-y-3 rounded-box border border-base-300 bg-base-200/40 p-3"
		onsubmit={(event) => {
			event.preventDefault();
			void reject();
		}}
	>
		{#if error}
			<div class="alert alert-error" role="alert"><span>{error}</span></div>
		{/if}

		<div class="fieldset">
			<label class="fieldset-legend" for="task-reject-feedback">Feedback</label>
			<textarea
				id="task-reject-feedback"
				class="textarea textarea-bordered min-h-20 w-full"
				placeholder="What the bee should do differently"
				bind:value={feedback}
			></textarea>
			<span class="fieldset-label">
				Publishes your feedback only. It does not start a rework task — for that, use Request
				changes on the merge preview.
			</span>
		</div>

		<div class="flex items-center gap-2">
			<button type="submit" class="btn btn-error btn-sm" disabled={!rejectReady}>
				{submitting === 'reject' ? 'Rejecting…' : 'Reject'}
			</button>
			<button type="button" class="btn btn-ghost btn-sm" onclick={reset}>Cancel</button>
		</div>
	</form>
{/if}
