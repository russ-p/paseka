import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import SectionHarness from './SectionHarness.svelte';

describe('Section', () => {
	it('uses a section element with a heading when it is not collapsible', () => {
		const { container } = render(SectionHarness, { title: 'Tasks' });
		expect(container.querySelector('details')).toBeNull();
		expect(screen.getByRole('heading', { name: 'Tasks' })).toBeInTheDocument();
	});

	it('collapses to a details element only when asked', () => {
		const { container } = render(SectionHarness, { title: 'Worktree', collapsible: true, open: false });
		const details = container.querySelector('details');
		expect(details).toBeInTheDocument();
		expect(details).not.toHaveAttribute('open');
	});

	it('opens a collapsible section by default so first-time content is visible', () => {
		const { container } = render(SectionHarness, { title: 'Worktree', collapsible: true });
		expect(container.querySelector('details')).toHaveAttribute('open');
	});

	it('puts the note beside the title and omits it when absent', () => {
		const { unmount } = render(SectionHarness, { note: '2 in the comb' });
		expect(screen.getByText('2 in the comb')).toBeInTheDocument();
		unmount();

		render(SectionHarness);
		expect(screen.queryByText('2 in the comb')).not.toBeInTheDocument();
	});

	it('keeps header actions on a plain section', async () => {
		const user = userEvent.setup();
		render(SectionHarness);
		const action = screen.getByRole('button', { name: 'Load more' });
		expect(action.closest('details')).toBeNull();
		await user.click(action);
		expect(action).toBeInTheDocument();
	});

	it('keeps header actions visible on a collapsed section without toggling it', async () => {
		const user = userEvent.setup();
		const { container } = render(SectionHarness, { collapsible: true, open: false });
		const details = container.querySelector('details') as HTMLDetailsElement;

		await user.click(screen.getByRole('button', { name: 'Load more' }));
		expect(details.open).toBe(false);
	});

	it('toggles a collapsed section from its summary', async () => {
		const user = userEvent.setup();
		const { container } = render(SectionHarness, { collapsible: true, open: false });
		const details = container.querySelector('details') as HTMLDetailsElement;

		await user.click(screen.getByText('Trail artifacts'));
		expect(details.open).toBe(true);
	});
});

describe('Section grid span', () => {
	it('applies an extra class to a plain block', () => {
		const { container } = render(SectionHarness, { class: 'lg:col-span-2' });
		expect(container.querySelector('section')?.className).toContain('lg:col-span-2');
	});

	it('applies an extra class to a collapsible block too', () => {
		const { container } = render(SectionHarness, { class: 'lg:col-span-2', collapsible: true });
		expect(container.querySelector('details')?.className).toContain('lg:col-span-2');
	});

	it('adds nothing when no span was asked for', () => {
		const { container } = render(SectionHarness);
		expect(container.querySelector('section')?.className).not.toContain('col-span');
	});
});
