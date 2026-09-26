<script lang="ts">
	import { AlertTriangle, FileText } from 'lucide-svelte';
	import { buildMergeDiffFiles, filterDiffFiles, rowAnchor, type DiffFile, type DiffRow } from '$lib/diff';
	import type { MergeDiff } from '$lib/api/types';

	let {
		diff,
		onpickline
	}: {
		diff: MergeDiff;
		/**
		 * A click on a line that can carry a note. `null` for a row that is not a
		 * line in the file — a hunk header or a mode change — which is what lets the
		 * comments panel be a plain consumer instead of scraping this markup.
		 */
		onpickline?: (anchor: { path: string; side: 'old' | 'new'; line: number; snippet: string }) => void;
	} = $props();

	let filter = $state('');
	/** The file whose section the body is scrolled to, as a path rather than an index. */
	let selectedPath = $state('');

	const files = $derived(buildMergeDiffFiles(diff));
	const visible = $derived(filterDiffFiles(files, filter));
	const selected = $derived(files.find((file) => file.path === selectedPath) ?? files[0] ?? null);

	$effect(() => {
		// A new patch starts at its first file: carrying a selection across traces
		// would scroll a review to a path the other trail never had.
		selectedPath = files[0]?.path ?? '';
	});

	/**
	 * The list filters, the body does not. Hiding a file's section would make the
	 * line numbers a comment anchors to jump around, and the file list is a
	 * navigation aid rather than the content.
	 *
	 * The legacy walked the list with `j`/`k` from a `tabindex="0"` div. A focusable
	 * div is not a control — it is a `<div>` that swallowed Tab — so this uses real
	 * buttons, which the keyboard reaches and Enter activates on its own. The
	 * legacy also marked the `<ul>` as a listbox while leaving every `<li>` without
	 * a role, so no option was ever announced; a nav of buttons has no such gap.
	 */
	function select(path: string): void {
		selectedPath = path;
		document.getElementById(diffSectionId(path))?.scrollIntoView({ block: 'start' });
	}

	function diffSectionId(path: string): string {
		// A path can hold characters that are not valid in an id fragment, so the
		// id is derived rather than assumed.
		return `diff-file-${encodeURIComponent(path)}`;
	}

	const tone: Record<DiffRow['kind'], string> = {
		meta: 'text-base-content/50',
		hunk: 'text-info',
		binary: 'text-warning',
		truncated: 'text-warning',
		add: 'bg-success/10',
		remove: 'bg-error/10',
		context: ''
	};
</script>

{#if diff.missingWorktree}
	<p class="text-base-content/70">
		No branch for this trail on this machine, so there is nothing to preview. The worktree is gone or
		was never created.
	</p>
{:else if diff.empty || !diff.diff}
	<p class="text-base-content/70">
		No changes between <span class="font-mono">{diff.defaultBranch}</span> and
		<span class="font-mono">{diff.branch}</span>. Nothing to merge.
	</p>
{:else if (diff.originBehindCount ?? 0) > 0}
	<div class="alert alert-warning" role="alert">
		<AlertTriangle class="h-5 w-5" strokeWidth={2.5} />
		<span>
			The local <span class="font-mono">{diff.defaultBranch}</span> is {diff.originBehindCount}
			commit(s) behind origin. A merge here may conflict with those commits.
		</span>
	</div>
{/if}

{#if diff.missingWorktree || diff.empty || !diff.diff}
	<!-- Nothing to show, and the notice above is the whole story. -->
{:else}
	<div class="space-y-2">
		{#if diff.truncated}
			<div class="alert alert-warning" role="alert">
				<AlertTriangle class="h-5 w-5" strokeWidth={2.5} />
				<span>
					Diff truncated at the server’s size cap — open the worktree locally for the full patch.
				</span>
			</div>
		{/if}

		<label class="input input-sm flex w-full items-center gap-2">
			<FileText class="h-4 w-4 text-base-content/40" strokeWidth={2.5} />
			<input
				type="search"
				class="grow"
				placeholder="Filter files by path"
				aria-label="Filter files by path"
				bind:value={filter}
			/>
		</label>

		<div class="grid gap-3 md:grid-cols-[16rem_1fr]">
			<nav class="max-h-[32rem] overflow-y-auto" aria-label="Changed files">
				{#if visible.length === 0}
					<p class="px-2 py-1 text-xs text-base-content/50">No files match the filter.</p>
				{:else}
					<ul class="space-y-0.5">
						{#each visible as file (file.path)}
							<li>
								<button
									type="button"
									class="block w-full rounded px-2 py-1 text-left text-xs hover:bg-base-200"
									class:bg-base-200={file.path === selected?.path}
									aria-current={file.path === selected?.path ? 'true' : undefined}
									onclick={() => select(file.path)}
								>
									<!-- Wrapped, not truncated. Two files called `config.go` and
									     `config_test.go` both truncated to `internal/gate/telegram/con…`,
									     which is the one thing a diff file list must not do: a
									     reviewer cannot tell the files apart. The stat moves to its
									     own line so the path gets the full width. -->
									<span class="block font-mono break-all">{file.path}</span>
									{#if file.statLabel}
										<span class="mt-0.5 block font-mono text-base-content/50">
											{file.statLabel}
											{#if file.truncated}<span class="text-warning"> · trunc</span>{/if}
											{#if file.binary}<span class="text-warning"> · bin</span>{/if}
										</span>
									{/if}
								</button>
							</li>
						{/each}
					</ul>
				{/if}
			</nav>

			<div
				class="max-h-[32rem] overflow-auto rounded-box border border-base-300 bg-base-100"
				aria-label="Merge diff"
			>
				{#each files as file (file.path)}
					{@const anchorFor = (row: DiffRow) => rowAnchor(file, row)}
					<section id={diffSectionId(file.path)} data-path={file.path} aria-label={file.path}>
						<h3 class="sticky top-0 z-10 border-b border-base-300 bg-base-200 px-2 py-1 font-mono text-xs">
							{file.path}
							{#if file.oldPath}
								<span class="text-base-content/50">from {file.oldPath}</span>
							{/if}
						</h3>
						{#if file.binary}
							<p class="px-2 py-1 text-xs text-base-content/60">
								Binary file — there is no text diff to show.
							</p>
						{:else}
							<table class="w-full font-mono text-xs">
								<tbody>
									{#each file.rows as row, index (`${file.path}-${index}`)}
										{@const anchor = anchorFor(row)}
										<tr class={tone[row.kind]}>
											<td class="w-10 select-none px-1 text-right align-top text-base-content/30">
												{row.kind === 'add' || row.kind === 'remove' || row.kind === 'context'
													? (row.oldLine ?? '')
													: ''}
											</td>
											<td class="w-10 select-none px-1 text-right align-top text-base-content/30">
												{row.kind === 'add' || row.kind === 'remove' || row.kind === 'context'
													? (row.newLine ?? '')
													: ''}
											</td>
											<td class="w-4 select-none text-center align-top text-base-content/40">
												{row.kind === 'add' ? '+' : row.kind === 'remove' ? '−' : ''}
											</td>
											<td
												class="px-1 align-top whitespace-pre-wrap {anchor !== null && onpickline
													? 'cursor-pointer decoration-dotted underline-offset-4 decoration-base-content/30'
													: ''}"
												onclick={() => {
													if (anchor && onpickline) onpickline(anchor);
												}}
											>
												<!-- The row's own text, escaped by Svelte. The legacy handed
												     diff2html's HTML to innerHTML here. -->
												{#if row.kind === 'meta' || row.kind === 'hunk' || row.kind === 'binary' || row.kind === 'truncated'}
													{row.text}
												{:else}
													{row.text === '' ? ' ' : row.text}
												{/if}
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						{/if}
					</section>
				{/each}
			</div>
		</div>
	</div>
{/if}
