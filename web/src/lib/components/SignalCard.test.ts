import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import SignalCard from './SignalCard.svelte';
import { insightHighlight } from '../../tests/fixtures';

describe('SignalCard', () => {
	it('shows the summary and the source line without a redundant badge', () => {
		render(SignalCard, { insight: insightHighlight() });

		expect(screen.getByText('The seam is now the only place that knows about providers.')).toBeInTheDocument();
		expect(screen.getByText(/^INSIGHT · builder · /)).toBeInTheDocument();
		expect(screen.getByText('trace-01a0bd6963faa14f')).toBeInTheDocument();
		expect(screen.getByText('run-01')).toBeInTheDocument();
		expect(screen.queryByRole('alert')).not.toBeInTheDocument();
	});

	it('badges the severity when the server sends one', () => {
		render(SignalCard, { insight: insightHighlight({ severity: 'warning' }) });
		expect(screen.getByText('warning')).toBeInTheDocument();
	});

	it('links the summary only when a target surface exists', () => {
		const { unmount } = render(SignalCard, {
			insight: insightHighlight(),
			href: '/next/timeline'
		});
		expect(
			screen.getByRole('link', { name: 'The seam is now the only place that knows about providers.' })
		).toHaveAttribute('href', '/next/timeline');
		unmount();

		render(SignalCard, { insight: insightHighlight() });
		expect(screen.queryByRole('link')).not.toBeInTheDocument();
	});
});
