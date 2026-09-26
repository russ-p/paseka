import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import type { Component } from 'svelte';
import DataTable from './DataTable.svelte';
import type { DataColumn } from './DataTable.svelte';
import { runSummary } from '../../tests/fixtures';
import type { RunSummary } from '$lib/api/types';

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
