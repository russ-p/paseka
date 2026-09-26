import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
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
	rowKey: (run: RunSummary) => string;
	label: string;
	pageSize?: number;
	loading?: boolean;
	emptyMessage?: string;
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
