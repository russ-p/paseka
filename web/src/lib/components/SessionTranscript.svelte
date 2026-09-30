<script lang="ts">
	import { ArrowDown } from 'lucide-svelte';
	import type { TranscriptEntry } from '$lib/api/types';

	let {
		lines,
		loading = false,
		error = '',
		/** How many lines the store is holding, which is more than this window shows. */
		dropped = 0
	}: {
		lines: TranscriptEntry[];
		loading?: boolean;
		error?: string;
		dropped?: number;
	} = $props();

	let viewport = $state<HTMLDivElement | null>(null);
	/** Whether the view is pinned to the newest line. */
	let following = $state(true);

	/**
	 * Follow the tail, but only while the reader is already at it.
	 *
	 * The legacy set `scrollTop = scrollHeight` on every tick, so a transcript could
	 * never be scrolled up: the moment an operator tried to read a line, the next poll
	 * yanked it away. Following the tail is what you want while watching a session
	 * finish, and following the reader is what you want the instant they reach for the
	 * history — so the newest line wins only until the reader says otherwise.
	 */
	function atBottom(): boolean {
		if (!viewport) return true;
		return viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < 24;
	}

	$effect(() => {
		// Read the length so this re-runs per appended page, and the tail position only
		// after the DOM has the new lines in it.
		lines.length;
		if (following) {
			void Promise.resolve().then(() => {
				if (viewport) viewport.scrollTop = viewport.scrollHeight;
			});
		}
	});

	/**
	 * The role label is coloured by role, but the tint is decoration rather than the
	 * message: the word itself says who spoke, so a reader who cannot see the colour
	 * loses nothing.
	 */
	function roleTone(role: string): string {
		if (role === 'agent') return 'text-success';
		if (role === 'system') return 'text-info';
		return 'text-base-content/40';
	}

	function onScroll(): void {
		following = atBottom();
	}

	function jumpToLatest(): void {
		following = true;
		if (viewport) viewport.scrollTop = viewport.scrollHeight;
	}
</script>

<section class="space-y-2" aria-label="Transcript">
	{#if error}
		<div class="alert alert-error" role="alert"><span>{error}</span></div>
	{/if}

	{#if lines.length === 0 && !loading}
		<p class="text-sm text-base-content/60">
			Nothing was written to this session's transcript. The agent's terminal output is not recorded
			here — only the lines it normalised into <code class="font-mono">transcript.ndjson</code>.
		</p>
	{:else}
		<div class="relative">
			<!--
			     `tabindex="0"` on a scrollable region is not a keyboard-trap workaround:
			     it is the only way a keyboard can scroll it, because a scrollable `<div>` is
			     otherwise unreachable with the arrow keys — the WCAG 2.1.1 failure the
			     console fixed on the legacy's `tabindex="0"` *diff* div, which was focusable
			     because it pretended to be a control. This one is focusable because it
			     scrolls, which is the case the rule's authors did not model.

			     `region` rather than `log`: `log` implies `aria-live="polite"`, and
			     announcing every line an agent writes while someone is reading history is
			     the opposite of helpful.
			-->
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<div
				bind:this={viewport}
				class="max-h-[32rem] min-h-48 overflow-auto rounded-box border border-base-300 bg-base-100 p-2 font-mono text-xs"
				onscroll={onScroll}
				tabindex="0"
				role="region"
				aria-label="Transcript lines"
			>
				{#if dropped > 0}
					<p class="mb-1 text-base-content/50">
						… {dropped} earlier {dropped === 1 ? 'line is' : 'lines are'} not shown. The full
						transcript is in the run directory below.
					</p>
				{/if}
				{#each lines as line, index (`${line.at}-${index}`)}
					<!--
					     `whitespace-pre`, and the region scrolls sideways. What is in here is a
					     terminal: an interactive session's `transcript.ndjson` is normalised
					     screen output, box drawing and all, and wrapping it folds every aligned
					     column into a staircase. The real session here shows a slash-command
					     menu whose two columns are only readable unwrapped.
					 -->
					<div class="flex gap-3">
						<span class="w-12 shrink-0 select-none text-right {roleTone(line.role)}">{line.role}</span>
						<span class="min-w-0 flex-1">{line.content}</span>
					</div>
				{/each}
			</div>
			{#if !following}
				<!-- Reading history is a mode the reader chose, so it gets a way back. -->
				<button
					type="button"
					class="btn btn-primary btn-xs absolute right-3 bottom-3 shadow"
					onclick={jumpToLatest}
				>
					<ArrowDown class="h-3.5 w-3.5" strokeWidth={2.5} />
					Latest
				</button>
			{/if}
		</div>
	{/if}

	{#if loading}
		<p class="text-xs text-base-content/50" aria-live="polite">Reading…</p>
	{/if}
</section>
