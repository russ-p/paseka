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
			trace: traceSummary({ hasActive: false, hasFailures: true })
		});
		expect(screen.getByText('failures')).toHaveClass('badge-error');
		failed.unmount();

		const idle = render(TraceRow, {
			trace: traceSummary({ hasActive: false, hasFailures: false, runCount: 2 })
		});
		expect(screen.getByText('2 runs')).toHaveClass('badge-neutral');
	});

	it('links the title to a detail route only when one is given', () => {
		const { unmount } = render(TraceRow, { trace: traceSummary(), href: '/next/traces/x' });
		expect(screen.getByRole('link', { name: 'Refactor the adapter seam' })).toHaveAttribute(
			'href',
			'/next/traces/x'
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
