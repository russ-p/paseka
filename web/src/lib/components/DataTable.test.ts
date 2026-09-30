import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';
import type { Component } from 'svelte';
import DataTable from './DataTable.svelte';
import type { DataColumn } from './DataTable.svelte';
import { runSummary, traceSummary } from '../../tests/fixtures';
import { tracePrimaryLabel, traceState, traceStateLabel } from '$lib/format';
import type { RunSummary, TraceSummary } from '$lib/api/types';

const columns: DataColumn<RunSummary>[] = [
	{ key: 'bee', label: 'Bee', text: (run) => run.bee },
	{ key: 'state', label: 'State', text: (run) => run.state },
	{ key: 'trace', label: 'Trace', text: (run) => run.traceId, secondary: true }
];

const rows = [
	runSummary({ agentId: 'a', bee: 'builder', state: 'failed' }),
	runSummary({ agentId: 'b', bee: 'guard', state: 'cancelled' }),
	runSummary({ agentId: 'c', bee: 'scout', state: 'failed' })
];

interface TableProps {
	columns: DataColumn<RunSummary>[];
	rows: RunSummary[];
	rowKey: (row: RunSummary) => string;
	label: string;
	pageSize?: number;
	loading?: boolean;
	emptyMessage?: string;
	stateKey?: string;
}

/** `render` erases a generic component's type parameter to `unknown`, so bind `T` here. */
const Table = DataTable as unknown as Component<TableProps>;
const TraceTable = DataTable as unknown as Component<{
	columns: DataColumn<TraceSummary>[];
	rows: TraceSummary[];
	rowKey: (row: TraceSummary) => string;
	label: string;
}>;

function renderTable(overrides: Partial<TableProps> = {}) {
	return render(Table, {
		columns,
		rows,
		rowKey: (run: RunSummary) => run.agentId,
		label: 'Failed runs',
		...overrides
	});
}

describe('DataTable', () => {
	it('renders a header plus one row per record', () => {
		renderTable();

		expect(screen.getByRole('table')).toBeInTheDocument();
		expect(screen.getByRole('columnheader', { name: 'Bee' })).toBeInTheDocument();
		expect(screen.getAllByRole('row')).toHaveLength(4);
		expect(screen.getByText('builder')).toBeInTheDocument();
		expect(screen.getByText('cancelled')).toBeInTheDocument();
	});

	it('filters across every column projection and resets pagination', async () => {
		const user = userEvent.setup();
		renderTable({ pageSize: 1 });

		expect(screen.getByText('1–1 of 3')).toBeInTheDocument();

		await user.type(screen.getByRole('searchbox', { name: 'Filter' }), 'guard');
		expect(screen.getAllByRole('row')).toHaveLength(2);
		expect(screen.getByText('1–1 of 1')).toBeInTheDocument();
		expect(screen.queryByText('builder')).not.toBeInTheDocument();

		await user.clear(screen.getByRole('searchbox', { name: 'Filter' }));
		await user.type(screen.getByRole('searchbox', { name: 'Filter' }), 'no-such-bee');
		expect(screen.getByText('Nothing to show.')).toBeInTheDocument();
		expect(screen.getByText('0 of 0')).toBeInTheDocument();
	});

	it('paginates and disables the edge buttons', async () => {
		const user = userEvent.setup();
		renderTable({ pageSize: 2 });

		const previous = screen.getByRole('button', { name: 'Previous' });
		const next = screen.getByRole('button', { name: 'Next' });
		expect(previous).toBeDisabled();
		expect(screen.getByText('1–2 of 3')).toBeInTheDocument();

		await user.click(next);

		expect(screen.getByText('3–3 of 3')).toBeInTheDocument();
		expect(next).toBeDisabled();
		expect(previous).toBeEnabled();
	});

	it('skeletons the body while loading', () => {
		const { container } = renderTable({ loading: true });
		expect(container.querySelectorAll('.skeleton').length).toBe(columns.length * 3);
	});

	it('hides secondary columns below the 768px breakpoint', () => {
		renderTable();
		expect(screen.getByRole('columnheader', { name: 'Trace' }).className).toContain('hidden');
		expect(screen.getByRole('columnheader', { name: 'Bee' }).className).not.toContain('hidden');
	});

	it('renders a custom empty message for an empty result', () => {
		renderTable({ rows: [], emptyMessage: 'No failed runs.' });
		expect(screen.getByText('No failed runs.')).toBeInTheDocument();
	});
});

describe('DataTable list state', () => {
	/** The deep link an operator arrives on, which is the whole reason state is in the URL. */
	function arriveAt(query: string): void {
		window.history.replaceState(null, '', `/next/traces${query}`);
	}

	function searchBox(): HTMLElement {
		return screen.getByRole('searchbox', { name: 'Filter' });
	}

	it('seeds the filter and the page from the query, so a link is the list you meant', () => {
		arriveAt('?q=guard&page=1');
		renderTable({ pageSize: 1 });

		expect(searchBox()).toHaveValue('guard');
		// Page one of a one-row result, which is only reachable if the seed was read.
		expect(screen.getByText('cancelled')).toBeInTheDocument();
		expect(screen.queryByText('builder')).not.toBeInTheDocument();
	});

	it('writes the filter into the query, so a filtered list is a link', async () => {
		const user = userEvent.setup();
		renderTable({ pageSize: 1 });

		await user.type(searchBox(), 'guard');

		expect(window.location.search).toBe('?q=guard');
	});

	it('writes the page, and drops both params once the view is the default again', async () => {
		const user = userEvent.setup();
		renderTable({ pageSize: 2 });
		expect(window.location.search).toBe('');

		await user.click(screen.getByRole('button', { name: 'Next' }));
		expect(window.location.search).toBe('?page=1');

		await user.click(screen.getByRole('button', { name: 'Previous' }));
		expect(window.location.search).toBe('');
	});

	it('resets to the first page when the filter changes, and says so in the query', async () => {
		const user = userEvent.setup();
		renderTable({ pageSize: 1 });
		await user.click(screen.getByRole('button', { name: 'Next' }));
		expect(window.location.search).toBe('?page=1');

		await user.type(searchBox(), 'guard');

		// One row left, and page one again: the page the operator was on is gone with the
		// rows that filled it. `page` drops out of the query because page one is the
		// default, which is what a stale `?page=1` here would otherwise be read back as.
		expect(window.location.search).toBe('?q=guard');
		expect(screen.getByText('1–1 of 1')).toBeInTheDocument();
	});

	it('publishes nothing while it has no rows, so a shared page link survives its own arrival', async () => {
		arriveAt('?page=1');
		// A list route on a cold load: the rows are on the way, so the page under the
		// seed is derived from nothing. Publishing that is what reached `replaceState`
		// before the router could take it — the write is a no-op the router refuses, and
		// it is the one arrival that asks for it, because a default view spends nothing.
		const { rerender } = renderTable({ rows: [], pageSize: 2, loading: true });

		expect(window.location.search).toBe('?page=1');

		rerender({ columns, rows, rowKey: (run) => run.agentId, label: 'Runs', pageSize: 2 });

		// The rows make the page knowable, and it is the page the link named.
		await waitFor(() => expect(screen.getByText('3–3 of 3')).toBeInTheDocument());
		expect(window.location.search).toBe('?page=1');
	});

	it('corrects a bookmarked page the rows cannot fill, rather than leaving it in the URL', () => {
		arriveAt('?page=9');
		renderTable({ pageSize: 2 });

		// `currentPage` clamps for display, and the write-back puts the truth in the bar
		// so the URL and the table under it cannot disagree.
		expect(screen.getByText('3–3 of 3')).toBeInTheDocument();
		expect(window.location.search).toBe('?page=1');
	});

	it('shows every row and no pager when the page size is zero, leaving the depth to the server', () => {
		renderTable({ pageSize: 0 });

		expect(screen.getAllByRole('row')).toHaveLength(4);
		expect(screen.queryByRole('button', { name: 'Next' })).not.toBeInTheDocument();
		expect(screen.queryByRole('button', { name: 'Previous' })).not.toBeInTheDocument();
		// No `1–3 of 3` either: the count a table prints is over the rows it holds, which
		// is not the number of rows the server has.
		expect(screen.queryByText(/of 3/)).not.toBeInTheDocument();
	});

	it('filters an unpaged table without paging it, and writes no page', async () => {
		const user = userEvent.setup();
		renderTable({ pageSize: 0 });

		await user.type(searchBox(), 'guard');

		expect(screen.getByText('cancelled')).toBeInTheDocument();
		expect(screen.queryByText('builder')).not.toBeInTheDocument();
		expect(window.location.search).toBe('?q=guard');
	});

	it('drops a seeded page it cannot honour, because an unpaged table has no page to land on', () => {
		arriveAt('?page=3');
		renderTable({ pageSize: 0 });

		// Three rows, no pages: the whole list is on screen, so the number the link
		// carried describes nothing this table can show and the bar is cleared of it.
		expect(screen.getAllByRole('row')).toHaveLength(4);
		expect(screen.queryByText(/of 3/)).not.toBeInTheDocument();
		expect(window.location.search).toBe('');
	});

	it('leaves somebody else’s query alone, because a param is not the table’s to claim', async () => {
		const user = userEvent.setup();
		arriveAt('?trace=trace-01a0bd6963faa14f');
		renderTable({ pageSize: 1 });

		await user.type(searchBox(), 'guard');

		expect(window.location.search).toBe('?trace=trace-01a0bd6963faa14f&q=guard');
	});

	it('namespaces both params under a state key, so a second table cannot fight this one', async () => {
		const user = userEvent.setup();
		window.history.replaceState(null, '', '/next/runs?runs.q=guard&runs.page=1&q=ignored');
		render(Table, {
			columns,
			rows,
			rowKey: (run: RunSummary) => run.agentId,
			label: 'Failed runs',
			pageSize: 1,
			stateKey: 'runs'
		});

		// Its own key is read and the unprefixed `q` beside it is somebody else's, or would
		// be if this route ever grew a second table. `runs.page=1` is dropped rather than
		// left behind, because one filtered row at a page size of one is page one and the
		// write-back writes the page on screen.
		expect(searchBox()).toHaveValue('guard');
		expect(window.location.search).toBe('?runs.q=guard&q=ignored');

		await user.clear(searchBox());
		// Only the keyed param goes: `q=ignored` was never this table's to write.
		expect(window.location.search).toBe('?q=ignored');
		expect(searchBox()).toHaveValue('');
	});

	it('ignores a page the URL cannot be trusted for', () => {
		arriveAt('?page=not-a-number');
		renderTable({ pageSize: 1 });

		expect(screen.getByText('1–1 of 3')).toBeInTheDocument();
	});
});

describe('DataTable cell intents', () => {
	const traceColumns: DataColumn<TraceSummary>[] = [
		{
			key: 'trace',
			label: 'Trace',
			text: (trace) => tracePrimaryLabel(trace),
			searchText: (trace) => `${trace.traceId} ${trace.standing ? 'standing' : ''}`,
			href: (trace) => `/next/traces/${trace.traceId}`,
			badge: (trace) => (trace.standing ? { status: 'standing', label: 'standing' } : null),
			grow: true
		},
		{
			key: 'state',
			label: 'State',
			text: (trace) => traceStateLabel(trace),
			badge: (trace) =>
				trace.hasActive || trace.hasFailures
					? { status: traceState(trace), label: traceState(trace) }
					: null
		}
	];
	const traceRows = [
		traceSummary({ traceId: 'trace-a', title: 'Alpha trail', hasActive: true }),
		traceSummary({ traceId: 'trace-b', title: 'Beta trail', standing: true, hasActive: false })
	];

	function renderTraces() {
		return render(TraceTable, {
			columns: traceColumns,
			rows: traceRows,
			rowKey: (trace: TraceSummary) => trace.traceId,
			label: 'Traces'
		});
	}

	it('links a cell and badges the same cell when both are declared', () => {
		renderTraces();

		expect(screen.getByRole('link', { name: 'Alpha trail' })).toHaveAttribute(
			'href',
			'/next/traces/trace-a'
		);
		expect(screen.getByText('standing')).toBeInTheDocument();
		expect(screen.getByText('active')).toBeInTheDocument();
	});

	it('leaves the cell empty when a declared badge has nothing to say', () => {
		renderTraces();

		const beta = screen.getByRole('link', { name: 'Beta trail' }).closest('tr');
		const cells = within(beta as HTMLElement).getAllByRole('cell');
		expect(cells[1]).toBeEmptyDOMElement();
	});

	it('matches the filter against terms the cell does not show', async () => {
		const user = userEvent.setup();
		renderTraces();

		await user.type(screen.getByLabelText('Filter'), 'trace-b');
		expect(screen.getByRole('link', { name: 'Beta trail' })).toBeInTheDocument();
		expect(screen.queryByRole('link', { name: 'Alpha trail' })).not.toBeInTheDocument();
	});

	it('matches the filter on a flag that only the badge shows', async () => {
		const user = userEvent.setup();
		renderTraces();

		await user.type(screen.getByLabelText('Filter'), 'standing');
		expect(screen.getByRole('link', { name: 'Beta trail' })).toBeInTheDocument();
		expect(screen.queryByRole('link', { name: 'Alpha trail' })).not.toBeInTheDocument();
	});

	it('gives the grow column the leftover width and pins it to one line', () => {
		const { container } = renderTraces();

		const header = container.querySelector('th');
		expect(header?.className).toContain('w-full');
		expect(header?.className).toContain('max-w-0');
		const cell = container.querySelector('tbody td');
		expect(cell?.className).toContain('whitespace-nowrap');
	});

	it('keeps every cell on one line so a table never doubles its row height', () => {
		const { container } = renderTraces();

		for (const cell of container.querySelectorAll('td')) {
			expect(cell.className).toContain('whitespace-nowrap');
		}
	});
});

describe('DataTable conditional cells', () => {
	interface Row {
		branch: string;
		traceId?: string;
		prUrl?: string;
	}
	/** An unregistered worktree: no trail to open, no pull request. */
	const rows: Row[] = [
		{ branch: 'paseka/trace-01' },
		{ branch: 'paseka/trace-02', traceId: 'trace-02', prUrl: 'https://example.test/2' }
	];
	const columnList: DataColumn<Row>[] = [
		{ key: 'branch', label: 'Branch', text: (row) => row.branch, mono: true, grow: true },
		{
			key: 'trace',
			label: 'Trace',
			text: (row) => row.traceId || '—',
			href: (row) => (row.traceId ? `/next/traces/${row.traceId}` : null)
		},
		{
			key: 'pr',
			label: 'Pull request',
			text: (row) => (row.prUrl ? 'open' : ''),
			href: (row) => row.prUrl ?? null,
			// The declared-but-null badge is what keeps the cell empty; without it the
			// link's fallback text would read as a dead `open`.
			badge: () => null
		}
	];

	function renderRows() {
		return render(Table as unknown as Component<Record<string, unknown>>, {
			columns: columnList,
			rows,
			rowKey: (row: Row) => row.branch,
			label: 'Worktrees'
		});
	}

	it('falls back to plain text when a row has nowhere to link', () => {
		renderRows();

		const orphan = screen.getByText('paseka/trace-01').closest('tr');
		const cells = within(orphan as HTMLElement).getAllByRole('cell');
		// A link to the empty string would be a promise the page cannot keep.
		expect(cells[1]).toHaveTextContent('—');
		expect(within(cells[1] as HTMLElement).queryByRole('link')).not.toBeInTheDocument();
	});

	it('keeps a cell empty when a declared badge declines and there is no link either', () => {
		renderRows();

		const orphan = screen.getByText('paseka/trace-01').closest('tr');
		const cells = within(orphan as HTMLElement).getAllByRole('cell');
		expect(cells[2]).toBeEmptyDOMElement();
	});

	it('links the rows that do have a target', () => {
		renderRows();

		expect(screen.getByRole('link', { name: 'trace-02' })).toHaveAttribute(
			'href',
			'/next/traces/trace-02'
		);
		expect(screen.getByRole('link', { name: 'open' })).toHaveAttribute('href', 'https://example.test/2');
	});

	it('renders a ref or sha in a monospace face', () => {
		renderRows();

		expect(screen.getByText('paseka/trace-01')).toHaveClass('font-mono');
	});

	it('truncates the grow column text, or it overflows its own cell', () => {
		const { container } = renderRows();

		const text = container.querySelector('tbody td span');
		expect(text?.className).toContain('truncate');
		expect(text?.className).toContain('block');
	});
});

describe('DataTable row actions', () => {
	interface Row {
		name: string;
		deletable: boolean;
	}

	const actionColumns: DataColumn<Row>[] = [
		{
			key: 'name',
			label: 'Branch',
			text: (row) => row.name,
			mono: true,
			action: (row) =>
				row.deletable
					? { label: 'Delete', kind: 'destructive', onselect: () => removed.push(row.name) }
					: null
		}
	];

	let removed: string[] = [];

	beforeEach(() => {
		removed = [];
	});

	function renderActions() {
		const rows: Row[] = [
			{ name: 'feature/one', deletable: true },
			{ name: 'paseka/trace-01', deletable: false }
		];
		return render(Table as unknown as Component<Record<string, unknown>>, {
			columns: actionColumns,
			rows,
			rowKey: (row: Row) => row.name,
			label: 'Branches'
		});
	}

	it('offers the action on the rows it applies to, and nothing on the rows it does not', () => {
		renderActions();

		// One control beside the value it acts on, named for the row: "Delete" alone in a
		// table of branches is a button nobody can act on.
		const button = screen.getByRole('button', { name: 'Delete feature/one' });
		expect(button.className).toContain('btn-error');
		expect(screen.getAllByRole('button', { name: /^Delete / })).toHaveLength(1);
	});

	it('runs the action against the row it sits in', async () => {
		const user = userEvent.setup();
		renderActions();

		await user.click(screen.getByRole('button', { name: 'Delete feature/one' }));

		expect(removed).toEqual(['feature/one']);
	});

	it('keeps a cell with no action as a plain cell', () => {
		const { container } = renderActions();

		const quiet = screen.getByText('paseka/trace-01').closest('td') as HTMLElement;
		// `flex` is added only where there is a control, so a cell nothing can act on
		// keeps the markup the empty-cell contract depends on.
		expect(quiet.className).not.toContain('flex');
		expect(container.querySelectorAll('tbody td').length).toBe(2);
	});
});
