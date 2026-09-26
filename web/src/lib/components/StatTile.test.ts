import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { createRawSnippet } from 'svelte';
import StatTile from './StatTile.svelte';

describe('StatTile', () => {
	it('renders the label and the value', () => {
		render(StatTile, { label: 'Active worktrees', value: 2 });

		expect(screen.getByText('Active worktrees')).toBeInTheDocument();
		expect(screen.getByText('2')).toBeInTheDocument();
	});

	it('keeps the full hint text for a truncated value line', () => {
		render(StatTile, { label: 'NATS', value: 'not configured', hint: ['transport is optional'] });

		expect(screen.getByText('not configured')).toBeInTheDocument();
		expect(screen.getByText('transport is optional')).toBeInTheDocument();
	});

	it('renders the content snippet in place of the value', () => {
		const badge = createRawSnippet(() => ({ render: () => '<span>connected</span>' }));
		render(StatTile, { label: 'NATS', value: 'ok', children: badge });
		expect(screen.getByText('connected')).toBeInTheDocument();
		expect(screen.queryByText('ok')).not.toBeInTheDocument();
	});

	it('links to a related route when one is given', () => {
		render(StatTile, { label: 'Active traces', value: 1, href: '/next/traces' });
		expect(screen.getByRole('link', { name: 'open' })).toHaveAttribute('href', '/next/traces');
	});

	it('forwards the id and merges extra classes so a tile can span a grid', () => {
		render(StatTile, {
			id: 'dashboard-task-counts',
			label: 'Task counts',
			value: 3,
			class: 'lg:col-span-2'
		});

		const tile = document.querySelector('#dashboard-task-counts');
		expect(tile).toHaveClass('stat', 'rounded-box', 'lg:col-span-2');
	});

	it('keeps a truncating value readable through its hint', () => {
		const { container } = render(StatTile, {
			label: 'Active worktrees',
			value: 12,
			hint: ['Isolated checkouts under .paseka/worktrees.']
		});

		expect(container.querySelector('.truncate')).toHaveTextContent('12');
		// The popover is portaled to `body`, so it is not inside `container`.
		expect(document.body).toHaveTextContent('Isolated checkouts under .paseka/worktrees.');
	});
});
