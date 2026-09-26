<script lang="ts">
	import { base } from '$app/paths';
	import { approveTask, rejectTask } from '$lib/api/client';
	import type { TaskDetail } from '$lib/api/types';

	let {
		task,
		onsettled
	}: {
		task: TaskDetail;
		/** Fired after an approve or reject lands, so the caller can refresh and toast. */
		onsettled?: (outcome: { tone: 'success' | 'warning'; message: string }) => void;
	} = $props();

	/**
	 * The PR fields are only asked for once the operator opens Approve, and only
	 * shown at all when the task was actually delivered as a pull request — a task
	 * delivered as a merge commit has nothing to title. `open` is false by default
	 * for the same reason the run's task body is folded: the common action is
	 * approve-or-read, and a form of six fields above the fold competes with it.
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

	const isPR = $derived(task.delivery === 'pr' || task.pullRequest?.url !== undefined);
	const approveReady = $derived(submitting === null && summary.trim() !== '' && (isPR === false || prTitle.trim() !== ''));
	const rejectReady = $derived(submitting === null && feedback.trim() !== '');

	async function approve(): Promise<void> {
		if (!approveReady) return;
		submitting = 'approve';
		error = '';
		try {
			const result = await approveTask(task.traceId, task.taskId, {
				summary: summary.trim(),
				...(mergeMessage.trim() === '' ? {} : { mergeMessage: mergeMessage.trim() }),
				...(isPR ? { prTitle: prTitle.trim(), prBody: prBody.trim(), draft, runHooks } : {})
			});
			onsettled?.({
				tone: 'success',
				message:
					result.message ??
					(result.prUrl ? `Approved, ${result.prUrl}` : `Approved ${task.taskId}`)
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
				message: result.message ?? (result.reworkTaskId ? `Rework task ${result.reworkTaskId} created` : `Rejected ${task.taskId}`)
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

<!-- Reject stays minimal: a line of feedback the bee gets as its rework input.
     Everything else the operator might want to say belongs in the PR or the
     commit, not in a second copy of the task body. -->
<div class="flex flex-wrap items-center gap-2">
	<button
		id="task-approve"
		type="button"
		class="btn btn-success btn-sm"
		onclick={() => {
			open = open === 'approve' ? null : 'approve';
			error = '';
		}}
	>
		<!-- An ellipsis says the verb opens something, which also keeps the toggle and
		     the submit from being two buttons with the same accessible name. -->
		Approve…
	</button>
	<button
		id="task-reject"
		type="button"
		class="btn btn-error btn-sm btn-outline"
		onclick={() => {
			open = open === 'reject' ? null : 'reject';
			error = '';
		}}
	>
		Request changes…
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
		</div>

		<div class="fieldset">
			<label class="fieldset-legend" for="task-approve-merge">Commit message</label>
			<input
				id="task-approve-merge"
				type="text"
				class="input input-bordered w-full font-mono"
				placeholder="optional — the bee's own summary is used when empty"
				bind:value={mergeMessage}
			/>
		</div>

		{#if isPR}
			<div class="fieldset">
				<label class="fieldset-legend" for="task-pr-title">PR title</label>
				<input
					id="task-pr-title"
					type="text"
					class="input input-bordered w-full"
					placeholder={task.prTitle || 'What this change does'}
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
					<span class="label">Draft</span>
				</label>
				<label class="flex cursor-pointer items-center gap-2">
					<input type="checkbox" class="checkbox checkbox-sm" bind:checked={runHooks} />
					<span class="label">Run hooks</span>
				</label>
			</div>
		{/if}

		<div class="flex items-center gap-2">
			<button type="submit" class="btn btn-success btn-sm" disabled={!approveReady}>
				{submitting === 'approve' ? 'Approving…' : 'Approve'}
			</button>
			<button type="button" class="btn btn-ghost btn-sm" onclick={reset}>Cancel</button>
			<span class="text-xs text-base-content/50">
				{isPR ? 'Merges this task and opens or updates its pull request.' : 'Merges this task.'}
			</span>
		</div>
	</form>
{:else if open === 'reject'}
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
				class="textarea textarea-bordered min-h-24 w-full"
				placeholder="What the bee should do differently. This becomes the rework task's body."
				bind:value={feedback}
			></textarea>
		</div>

		<div class="flex items-center gap-2">
			<button type="submit" class="btn btn-error btn-sm" disabled={!rejectReady}>
				{submitting === 'reject' ? 'Sending…' : 'Request changes'}
			</button>
			<button type="button" class="btn btn-ghost btn-sm" onclick={reset}>Cancel</button>
		</div>
	</form>
{/if}
