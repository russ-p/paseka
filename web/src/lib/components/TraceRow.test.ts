import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import TraceRow from './TraceRow.svelte';
import { traceSummary } from '../../tests/fixtures';

describe('TraceRow', () => {
	it('shows the title, meta line, standing badge, and state', () => {
		const { container } = render(TraceRow, {
			trace: traceSummary({ standing: true, runCount: 4, taskCount: 2 })
		});

		expect(screen.getByText('Refactor the adapter seam')).toBeInTheDocument();
		expect(container.querySelector('.truncate.text-xs')).toHaveTextContent(
			/^4 runs · 2 tasks · \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/
		);
		expect(screen.getByText('standing')).toBeInTheDocument();
		expect(screen.getByText('active')).toBeInTheDocument();
		expect(screen.getByText('trace-01a0bd6963faa14f')).toBeInTheDocument();
		expect(screen.getByText('builder')).toBeInTheDocument();
	});

	it('colours the state badge from the status map, not a local class', () => {
		const { unmount } = render(TraceRow, { trace: traceSummary({ hasActive: true }) });
		expect(screen.getByText('active')).toHaveClass('badge-info');
		unmount();

		const failed = render(TraceRow, {
			trace: traceSummary({ hasActive: false, hasFailures: true, openFailures: 2 })
		});
		expect(screen.getByText('2 failures')).toHaveClass('badge-error');
		failed.unmount();

		const recordOnly = render(TraceRow, {
			trace: traceSummary({
				hasActive: false,
				hasFailures: true,
				openFailures: 0,
				recordProblem: true
			})
		});
		expect(screen.getByText('record error')).toHaveClass('badge-error');
		recordOnly.unmount();

		// Both axes: the count is the badge and the integrity problem stays a
		// detail flag, never a second row badge.
		const both = render(TraceRow, {
			trace: traceSummary({
				hasActive: false,
				hasFailures: true,
				openFailures: 2,
				recordProblem: true
			})
		});
		expect(screen.getByText('2 failures')).toHaveClass('badge-error');
		expect(screen.queryByText('record error')).not.toBeInTheDocument();
		both.unmount();

		// A healed trail keeps the historical failure flag and shows no state badge:
		// the run count already lives on the meta line.
		render(TraceRow, {
			trace: traceSummary({ hasActive: false, hasFailures: true, openFailures: 0, runCount: 2 })
		});
		expect(screen.queryByText(/failure/)).not.toBeInTheDocument();
		expect(screen.queryByText('record error')).not.toBeInTheDocument();
		expect(screen.queryByText('2 runs')).not.toBeInTheDocument();
	});

	it('badges the trail\'s pause and pending count beside the trail, not as a run state', () => {
		render(TraceRow, {
			trace: traceSummary({
				hasActive: true,
				paused: true,
				pauseReason: 'Honey reserve exhausted',
				pendingCount: 2
			})
		});

		// The admission badges sit beside the trail, and the run-derived state keeps
		// its own word: a running run and an exhausted reserve are different facts.
		expect(screen.getByText('paused')).toHaveClass('badge-warning');
		expect(screen.getByText('2 pending')).toHaveClass('badge-warning');
		expect(screen.getByText('active')).toHaveClass('badge-info');
	});

	it('leaves a trail that is admitting work quiet', () => {
		render(TraceRow, { trace: traceSummary({ paused: false, pendingCount: 0 }) });

		expect(screen.queryByText('paused')).not.toBeInTheDocument();
		expect(screen.queryByText(/pending/)).not.toBeInTheDocument();
	});

	it('links the title to a detail route only when one is given', () => {
		const { unmount } = render(TraceRow, { trace: traceSummary(), href: '/traces/x' });
		expect(screen.getByRole('link', { name: 'Refactor the adapter seam' })).toHaveAttribute(
			'href',
			'/traces/x'
		);
		unmount();

		render(TraceRow, { trace: traceSummary() });
		expect(screen.queryByRole('link')).not.toBeInTheDocument();
	});

	it('falls back to the trace id without repeating it, and drops an absent summary', () => {
		render(TraceRow, { trace: traceSummary({ title: '', summary: undefined }) });

		expect(screen.getByText('trace-01a0bd6963faa14f')).toBeInTheDocument();
		expect(screen.queryByText(/only place that knows/)).not.toBeInTheDocument();
	});
});
