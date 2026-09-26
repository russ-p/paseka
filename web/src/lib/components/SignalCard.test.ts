import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import SignalCard from './SignalCard.svelte';
import { eventFeedItem, insightHighlight } from '../../tests/fixtures';

describe('SignalCard', () => {
	it('shows the summary and the source line without a redundant badge', () => {
		render(SignalCard, { signal: insightHighlight() });

		expect(screen.getByText('The seam is now the only place that knows about providers.')).toBeInTheDocument();
		expect(screen.getByText(/^INSIGHT · builder · /)).toBeInTheDocument();
		expect(screen.getByText('trace-01a0bd6963faa14f')).toBeInTheDocument();
		expect(screen.getByText('run-01')).toBeInTheDocument();
		expect(screen.queryByRole('alert')).not.toBeInTheDocument();
	});

	it('badges the severity when the server sends one', () => {
		render(SignalCard, { signal: insightHighlight({ severity: 'warning' }) });
		expect(screen.getByText('warning')).toBeInTheDocument();
	});

	it('links the summary only when a target surface exists', () => {
		const { unmount } = render(SignalCard, {
			signal: insightHighlight(),
			href: '/next/timeline'
		});
		expect(
			screen.getByRole('link', { name: 'The seam is now the only place that knows about providers.' })
		).toHaveAttribute('href', '/next/timeline');
		unmount();

		render(SignalCard, { signal: insightHighlight() });
		expect(screen.queryByRole('link')).not.toBeInTheDocument();
	});
});

describe('SignalCard feed rows', () => {
	it('names the contract and the payload kind on one line', () => {
		render(SignalCard, { signal: eventFeedItem({ type: 'MUTATION', payloadKind: 'file.edited' }) });

		expect(screen.getByText(/^MUTATION · file\.edited · builder · /)).toBeInTheDocument();
	});

	it('states the kind once when the contract and the payload agree', () => {
		render(SignalCard, { signal: eventFeedItem({ type: 'INSIGHT', payloadKind: 'INSIGHT' }) });

		expect(screen.getByText(/^INSIGHT · builder · /)).toBeInTheDocument();
	});

	it('drops the trace id inside a trail, where the header already names it', () => {
		const { unmount } = render(SignalCard, { signal: eventFeedItem() });
		expect(screen.getByText('trace-01a0bd6963faa14f')).toBeInTheDocument();
		unmount();

		render(SignalCard, { signal: eventFeedItem(), showTrace: false });
		expect(screen.queryByText('trace-01a0bd6963faa14f')).not.toBeInTheDocument();
		expect(screen.getByText('run-01')).toBeInTheDocument();
	});
});
