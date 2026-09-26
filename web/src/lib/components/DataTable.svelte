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
		/** Render the cell as a link to this destination. */
		href?: (row: T) => string;
		/** Render the cell as a `StatusBadge`, or nothing when the row returns `null`. */
		badge?: (row: T) => { status: string; label: string } | null;
		align?: 'left' | 'right';
		/**
		 * The column absorbs the leftover width, so a long cell truncates
		 * instead of pushing the table past the viewport. At most one per table.
		 */
		grow?: boolean;
		/** Hidden below 768px; the table must not force horizontal scroll on a phone. */
		secondary?: boolean;
	}
</script>

<script lang="ts" generics="T">
	import StatusBadge from '$lib/components/StatusBadge.svelte';

	let {
		columns,
		rows,
		rowKey,
		label,
		emptyMessage = 'Nothing to show.',
		filterLabel = 'Filter',
		filterPlaceholder = 'Filter',
		pageSize = 10,
		loading = false
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
	} = $props();

	let filter = $state('');
	let page = $state(0);

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
		filter;
		page = 0;
	});

	const alignClass = { left: 'text-left', right: 'text-right' } as const;

	/**
	 * Cells never wrap: a table is a scan surface, and a wrapped date or bee
	 * list doubles the row height. The one `grow` column takes the leftover
	 * width and truncates, so a long label can never push the table sideways.
	 */
	function cellClass(column: DataColumn<T>): string {
		return [
			'whitespace-nowrap',
			column.align === 'right' ? alignClass.right : alignClass.left,
			column.grow ? 'w-full max-w-0' : '',
			column.secondary ? 'hidden md:table-cell' : ''
		]
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
				bind:value={filter}
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

	<div class="overflow-x-hidden rounded-box border border-base-300 bg-base-100">
		<table class="table table-sm">
			<thead>
				<tr>
					{#each columns as column (column.key)}
						<th class={cellClass(column)} scope="col">
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
								<td class={cellClass(column)}>
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
								<td class={cellClass(column)}>
									{#if column.href}
										<div class="flex min-w-0 flex-wrap items-center gap-2">
											<a class="link truncate" href={column.href(row)}>{column.text(row)}</a>
											{#if cellBadge}
												<StatusBadge status={cellBadge.status} label={cellBadge.label} />
											{/if}
										</div>
									{:else if cellBadge}
										<StatusBadge status={cellBadge.status} label={cellBadge.label} />
									{:else if !column.badge}
										{column.text(row)}
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
