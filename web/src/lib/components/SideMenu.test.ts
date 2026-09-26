import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import SideMenu from './SideMenu.svelte';

describe('SideMenu', () => {
	it('opens, marks the active route, and restores focus on Escape', async () => {
		const user = userEvent.setup();
		render(SideMenu, { currentPath: '/next/traces' });
		const trigger = screen.getByRole('button', { name: 'Menu' });
		const panel = screen.getByLabelText('Console navigation');

		expect(trigger).toHaveAttribute('aria-expanded', 'false');
		expect(panel).toHaveAttribute('data-state', 'closed');

		await user.click(trigger);

		expect(trigger).toHaveAttribute('aria-expanded', 'true');
		expect(panel).toHaveAttribute('data-state', 'open');
		expect(screen.getByRole('link', { name: /Traces/ })).toHaveAttribute('aria-current', 'page');
		await waitFor(() => expect(screen.getByRole('link', { name: /Dashboard/ })).toHaveFocus());

		await user.keyboard('{Escape}');

		expect(panel).toHaveAttribute('data-state', 'closed');
		expect(trigger).toHaveFocus();
	});
});

describe('SideMenu active route', () => {
	async function openAt(currentPath: string) {
		const user = userEvent.setup();
		render(SideMenu, { currentPath });
		// The panel is inert while closed, so its links are not exposed until it opens.
		await user.click(screen.getByRole('button', { name: 'Menu' }));
	}

	it('keeps a parent entry active on a child route', async () => {
		await openAt('/next/traces/trace-01a0bd6963faa14f');

		expect(screen.getByRole('link', { name: /Traces/ })).toHaveAttribute('aria-current', 'page');
		expect(screen.getByRole('link', { name: /Dashboard/ })).not.toHaveAttribute('aria-current');
	});

	it('does not mistake a sibling route with a shared prefix for the entry', async () => {
		await openAt('/next/timeline');

		expect(screen.getByRole('link', { name: /Traces/ })).not.toHaveAttribute('aria-current');
		expect(screen.getByRole('link', { name: /Timeline/ })).toHaveAttribute('aria-current', 'page');
	});
});
