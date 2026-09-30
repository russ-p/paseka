import { render, screen, within } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import DetailRowHarness from './DetailRowHarness.svelte';

describe('DetailRow', () => {
	it('renders as a list item with the title, id line, and side badges', () => {
		const { container } = render(DetailRowHarness, {
			meta: 'run-01 · task-a1',
			dataKey: 'run-01'
		});

		const row = container.querySelector('li');
		expect(row).toHaveClass('detail-row');
		expect(row).toHaveAttribute('data-key', 'run-01');
		expect(screen.getByText('Prune implementation notes')).toBeInTheDocument();
		expect(screen.getByText('run-01 · task-a1').className).toContain('font-mono');
		expect(screen.getByText('announced')).toBeInTheDocument();
	});

	it('omits the id and detail lines when the caller has nothing to say', () => {
		const { container } = render(DetailRowHarness);
		expect(container.querySelectorAll('p')).toHaveLength(1);
	});

	it('shows the quiet third line and mirrors it into a hint for the truncated text', () => {
		const { container } = render(DetailRowHarness, { detail: '2026-09-20 09:15:06 · 29s' });
		const visible = within(container).getByText('2026-09-20 09:15:06 · 29s');
		expect(visible.tagName).toBe('P');
		// The popover is portaled to body and aria-hidden, so the row never nests a tooltip.
		const hint = document.body.querySelector('div[aria-hidden="true"][id^="paseka-hint-"]');
		expect(hint).toHaveTextContent('2026-09-20 09:15:06 · 29s');
	});

	it('links the title only when a destination surface exists', () => {
		const { unmount } = render(DetailRowHarness, { href: '/next/tasks/task-a1' });
		expect(screen.getByRole('link', { name: 'Prune implementation notes' })).toHaveAttribute(
			'href',
			'/next/tasks/task-a1'
		);
		unmount();

		render(DetailRowHarness);
		expect(screen.queryByRole('link')).not.toBeInTheDocument();
	});
});
