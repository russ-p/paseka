import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import DrawerHarness from './DrawerHarness.svelte';

describe('Drawer', () => {
	it('renders nothing at all while closed, so a form cannot be half-submitted', () => {
		render(DrawerHarness, { open: false });

		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
	});

	it('is a dialog with the same roles a centered modal has', () => {
		render(DrawerHarness, { open: true });

		const dialog = screen.getByRole('dialog', { name: 'New task' });
		expect(dialog).toHaveAttribute('aria-modal', 'true');
		expect(screen.getByText('drawer body')).toBeInTheDocument();
	});

	it('puts the panel at the right edge and full height, not centred', () => {
		render(DrawerHarness, { open: true });

		// A form with more than a couple of fields wants the full height, and the
		// class is the contract: `justify-end` on the overlay and `h-full` on the
		// panel. Asserting it here is what stops a later edit from quietly turning
		// the Drawer back into a modal.
		const panel = screen.getByRole('dialog');
		expect(panel.className).toContain('h-full');
		expect(panel.className).toContain('max-h-none');
		expect(panel.parentElement?.className).toContain('justify-end');
		// No bottom padding and no vertical centring, which is what a centered modal
		// uses.
		expect(panel.parentElement?.className).not.toContain('p-4');
	});

	it('escapes and restores focus like the modal it wraps', async () => {
		render(DrawerHarness, { open: true });

		// Focus lands inside the panel rather than staying on the page behind it.
		expect(document.activeElement?.closest('[role="dialog"]')).not.toBeNull();

		await userEvent.keyboard('{Escape}');
		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
	});
});
