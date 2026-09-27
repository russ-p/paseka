<script module lang="ts">
	export interface DataColumn<T> {
		key: string;
		label: string;
		/**
		 * Plain-text projection. Renders the cell when neither `href` nor `badge`
		 * is set, and always feeds the filter box, so every column should
		 * provide it.
		 */
		text: (row: T) => string;
		/**
		 * Extra terms that should match the filter but are not on screen — the
		 * trace id behind a titled row, a standing flag, an adapter name.
		 */
		searchText?: (row: T) => string;
		/**
		 * Render the cell as a link to this destination. A row with nowhere to
		 * go returns `null` and falls back to plain text, so a column can link
		 * the rows that have a target and leave the rest quiet.
		 */
		href?: (row: T) => string | null;
		/** Render the cell as a `StatusBadge`, or nothing when the row returns `null`. */
		badge?: (row: T) => { status: string; label: string } | null;
		/** Render the cell's text in a monospace face: refs, SHAs, paths. */
		mono?: boolean;
		align?: 'left' | 'right';
		/**
		 * The column absorbs the leftover width, so a long cell truncates
		 * instead of pushing the table past the viewport. At most one per table.
		 */
		grow?: boolean;
		/** Hidden below 768px; the table must not force horizontal scroll on a phone. */
		secondary?: boolean;
		/**
		 * One control rendered in this cell, beside its text — the same declarative shape
		 * as `href` and `badge`, and for the same reason: the column objects live in
		 * `<script>`, where a snippet cannot be built, and what that blocks is arbitrary
		 * markup rather than a callback.
		 *
		 * `null` for a row the action does not apply to, so a button appears on the rows
		 * that can take it and the table says which those are. The **route** owns the
		 * confirmation, not the table: a destructive action keeps its dialog beside the
		 * state that armed it.
		 */
		action?: (row: T) => { label: string; kind?: 'destructive'; onselect: () => void } | null;
	}
</script>

<script lang="ts" generics="T">
	import { untrack } from 'svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { readListState, writeListState } from '$lib/list-state';

	let {
		columns,
		rows,
		rowKey,
		label,
		emptyMessage = 'Nothing to show.',
		filterLabel = 'Filter',
		filterPlaceholder = 'Filter',
		pageSize = 10,
		loading = false,
		stateKey
	}: {
		columns: DataColumn<T>[];
		rows: T[];
		rowKey: (row: T) => string;
		/** Accessible name for the table region. */
		label: string;
		emptyMessage?: string;
		filterLabel?: string;
		filterPlaceholder?: string;
		pageSize?: number;
		loading?: boolean;
		/**
		 * Namespaces `?q=&page=` for a route that carries more than one table. Absent
		 * everywhere today, so the query is the plain one; the first page with a second
		 * table sets it.
		 */
		stateKey?: string;
	} = $props();

	/**
	 * The filter and the page are the URL's, seeded once on mount: that is what makes a
	 * list a link an operator can send, and what puts Back from a trail detail on the page
	 * they left rather than on page one. The untrack says the seed is one-time on purpose
	 * — a table that re-seeded on every URL write would feed its own write-back into
	 * itself.
	 */
	const seed = untrack(() => readListState(stateKey));
	let filter = $state(seed.q);
	let page = $state(seed.page);

	const filtered = $derived.by(() => {
		const needle = filter.trim().toLowerCase();
		if (needle === '') return rows;
		return rows.filter((row) =>
			columns.some((column) =>
				`${column.text(row)} ${column.searchText?.(row) ?? ''}`
					.toLowerCase()
					.includes(needle)
			)
		);
	});

	const pageCount = $derived(pageSize > 0 ? Math.max(1, Math.ceil(filtered.length / pageSize)) : 1);
	const currentPage = $derived(Math.min(page, pageCount - 1));
	const visible = $derived(pageSize > 0 ? filtered.slice(currentPage * pageSize, currentPage * pageSize + pageSize) : filtered);
	const rangeLabel = $derived(
		filtered.length === 0
			? '0 of 0'
			: `${currentPage * pageSize + 1}–${Math.min((currentPage + 1) * pageSize, filtered.length)} of ${filtered.length}`
	);

	$effect(() => {
		// The page written is the one on screen, so a bookmark whose page a poll has
		// invalidated corrects itself in the address bar rather than sitting there lying.
		writeListState({ q: filter, page: currentPage }, stateKey);
	});

	/**
	 * Typing narrows the rows, so the page it was on is usually gone; going back to page
	 * one is what "the list is now shorter" means. Done at the input rather than in an
	 * effect, because an effect that resets the page also runs on mount — which would
	 * throw away the very `?page=` this table was seeded with.
	 */
	function applyFilter(value: string): void {
		filter = value;
		page = 0;
	}

	const alignClass = { left: 'text-left', right: 'text-right' } as const;

	/**
	 * Cells never wrap: a table is a scan surface, and a wrapped date or bee
	 * list doubles the row height. A cell that runs out of room truncates
	 * instead, and the one `grow` column takes the leftover width so a long
	 * value can never push the table sideways. A cell carrying an action lays
	 * out as a row, so the control sits beside the value it acts on rather
	 * than under it — and only then, so a cell with nothing to press keeps the
	 * empty markup the badge contract depends on.
	 */
	function cellClass(column: DataColumn<T>, action: boolean): string {
		return [
			'whitespace-nowrap',
			column.align === 'right' ? alignClass.right : alignClass.left,
			column.grow ? 'w-full max-w-0' : '',
			column.secondary ? 'hidden md:table-cell' : '',
			action ? 'flex items-center gap-2' : ''
		]
			.filter(Boolean)
			.join(' ');
	}

	/** The `grow` cell's text truncates too, or it overflows its `max-w-0` cell. */
	function textClass(column: DataColumn<T>): string {
		return [column.grow ? 'block truncate' : '', column.mono ? 'font-mono' : '']
			.filter(Boolean)
			.join(' ');
	}
</script>

<section class="space-y-3" aria-label={label}>
	<div class="flex flex-wrap items-end justify-between gap-3">
		<label class="fieldset">
			<span class="fieldset-legend text-xs">{filterLabel}</span>
			<input
				type="search"
				class="input input-sm w-full max-w-xs"
				placeholder={filterPlaceholder}
				value={filter}
				data-list-filter
				oninput={(event) => applyFilter(event.currentTarget.value)}
			/>
		</label>
		{#if pageCount > 1}
			<div class="join">
				<button
					type="button"
					class="btn btn-sm join-item"
					disabled={currentPage === 0}
					onclick={() => (page = currentPage - 1)}
				>
					Previous
				</button>
				<span class="btn btn-sm join-item pointer-events-none">{rangeLabel}</span>
				<button
					type="button"
					class="btn btn-sm join-item"
					disabled={currentPage >= pageCount - 1}
					onclick={() => (page = currentPage + 1)}
				>
					Next
				</button>
			</div>
		{:else}
			<span class="text-xs text-base-content/50">{rangeLabel}</span>
		{/if}
	</div>

	<!-- Scrollable, not hidden. A wide table on a phone must not push the page
	     sideways, but clipping the last column outright makes a link in it
	     unreachable with no way to reveal it. The region scrolls instead, and the
	     page around it does not. -->
	<div class="overflow-x-auto rounded-box border border-base-300 bg-base-100">
		<table class="table table-sm">
			<thead>
				<tr>
					{#each columns as column (column.key)}
						<th class={cellClass(column, false)} scope="col">
							{column.label}
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#if loading}
					{#each Array.from({ length: Math.min(pageSize, 3) }) as _, index (index)}
						<tr>
							{#each columns as column (column.key)}
								<td class={cellClass(column, false)}>
									<span class="skeleton block h-3 w-full"></span>
								</td>
							{/each}
						</tr>
					{/each}
				{:else if visible.length === 0}
					<tr>
						<td colspan={columns.length} class="py-8 text-center text-base-content/60">
							{emptyMessage}
						</td>
					</tr>
				{:else}
						{#each visible as row (rowKey(row))}
							<tr>
								{#each columns as column (column.key)}
									{@const cellBadge = column.badge?.(row) ?? null}
									{@const cellHref = column.href?.(row) ?? null}
									{@const cellAction = column.action?.(row) ?? null}
									<td class={cellClass(column, cellAction !== null)}>
										{#if cellHref}
											<div class="flex min-w-0 flex-wrap items-center gap-2">
												<a class="link {column.grow ? 'truncate' : ''} {column.mono ? 'font-mono' : ''}" href={cellHref}
													>{column.text(row)}</a
												>
												{#if cellBadge}
													<StatusBadge status={cellBadge.status} label={cellBadge.label} />
												{/if}
											</div>
										{:else if cellBadge}
											<StatusBadge status={cellBadge.status} label={cellBadge.label} />
										{:else if !column.badge}
											<span class={textClass(column)}>{column.text(row)}</span>
										{/if}{#if cellAction}
											<!-- `aria-label` names the row, because "Delete" alone in a
											     table of branches is a control nobody can act on. The block
											     opens against the `{/if}` above on purpose: a newline
											     between them would leave a whitespace text node in every
											     cell, and a cell that renders nothing has to render
											     nothing. -->
											<button
												type="button"
												class="btn btn-xs {cellAction.kind === 'destructive' ? 'btn-error btn-outline' : 'btn-ghost'}"
												aria-label={`${cellAction.label} ${rowKey(row)}`}
												onclick={cellAction.onselect}
											>
												{cellAction.label}
											</button>
										{/if}
									</td>
								{/each}
							</tr>
						{/each}
				{/if}
			</tbody>
		</table>
	</div>
</section>
