import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import ModalHarness from './ModalHarness.svelte';

/** Tab is a browser-level move, so the trap is driven by keydown rather than `userEvent.tab`. */
async function pressTab(shift = false): Promise<void> {
	await fireEvent.keyDown(window, { key: 'Tab', shiftKey: shift });
}

describe('Modal', () => {
	it('renders nothing while closed, so a form behind it cannot be half-submitted', () => {
		render(ModalHarness, { open: false });

		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
		expect(screen.queryByRole('button', { name: 'Close dialog' })).not.toBeInTheDocument();
		// The page underneath is still reachable while the dialog is closed, which is
		// the whole point of the pattern: the list stays on screen.
		expect(screen.getByText('page behind the dialog')).toBeInTheDocument();
	});

	it('is a modal dialog named by its title, not a panel with a heading', async () => {
		render(ModalHarness, { open: false });
		await userEvent.click(screen.getByRole('button', { name: 'Open dialog' }));

		const dialog = screen.getByRole('dialog', { name: 'Delete merged leftover branches?' });
		expect(dialog).toHaveAttribute('aria-modal', 'true');
		// A dialog whose name came from a heading would read as a section to a screen
		// reader, and the whole surface is meant to be modal.
		expect(dialog).toHaveAttribute('tabindex', '-1');
	});

	it('puts the description under the title as body copy, and says nothing when it has none', async () => {
		const { rerender } = render(ModalHarness, {
			open: false,
			description: 'Local branches only — nothing is deleted on origin.'
		});
		await userEvent.click(screen.getByRole('button', { name: 'Open dialog' }));
		expect(
			screen.getByText('Local branches only — nothing is deleted on origin.')
		).toBeInTheDocument();

		await rerender({ open: true, description: undefined });
		// An empty paragraph under the title is a heading with nothing under it.
		expect(screen.getByRole('dialog').querySelector('.card-title + p')).toBeNull();
	});

	it('takes focus when it opens, so the keyboard is inside the dialog from the first key', async () => {
		render(ModalHarness, { open: false });
		await userEvent.click(screen.getByRole('button', { name: 'Open dialog' }));

		await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText('Branch')));
	});

	it('takes focus itself when it holds nothing to focus', async () => {
		// A comb preview has no control in it, and a dialog that focused nothing would
		// leave the keyboard on the page behind it with no way back in.
		render(ModalHarness, { open: false, bare: true, withFooter: false });
		await userEvent.click(screen.getByRole('button', { name: 'Open dialog' }));

		await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('dialog')));
	});

	it('closes on Escape and hands focus back to the button that opened it', async () => {
		render(ModalHarness, { open: false });
		const trigger = screen.getByRole('button', { name: 'Open dialog' });
		await userEvent.click(trigger);
		await waitFor(() => expect(document.activeElement).not.toBe(trigger));

		await userEvent.keyboard('{Escape}');

		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
		// Back on the trigger rather than at the top of the document, which is where a
		// reader who pressed Escape to back out of a dialog expects to be.
		expect(document.activeElement).toBe(trigger);
	});

	it('closes on a click outside the panel, which is how a dialog is dismissed without reading it', async () => {
		render(ModalHarness, { open: false });
		await userEvent.click(screen.getByRole('button', { name: 'Open dialog' }));

		await userEvent.click(screen.getByRole('button', { name: 'Close dialog' }));

		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
	});

	it('does not close on a click inside the panel', async () => {
		render(ModalHarness, { open: false });
		await userEvent.click(screen.getByRole('button', { name: 'Open dialog' }));

		await userEvent.click(
			screen.getByRole('heading', { name: 'Delete merged leftover branches?' })
		);

		expect(screen.getByRole('dialog')).toBeInTheDocument();
	});

	it('wraps from the last control to the first, so Tab cannot walk out of the dialog', async () => {
		render(ModalHarness, { open: false });
		await userEvent.click(screen.getByRole('button', { name: 'Open dialog' }));
		const first = screen.getByLabelText('Branch');
		const last = screen.getByRole('button', { name: 'Delete' });

		last.focus();
		await pressTab();

		expect(document.activeElement).toBe(first);
	});

	it('wraps backwards from the first control to the last', async () => {
		render(ModalHarness, { open: false });
		await userEvent.click(screen.getByRole('button', { name: 'Open dialog' }));
		const first = screen.getByLabelText('Branch');
		const last = screen.getByRole('button', { name: 'Delete' });

		first.focus();
		await pressTab(true);

		expect(document.activeElement).toBe(last);
	});

	it('leaves Tab alone in the middle, because a trap that hijacks every press is worse than none', async () => {
		render(ModalHarness, { open: false });
		await userEvent.click(screen.getByRole('button', { name: 'Open dialog' }));

		// jsdom has no browser-level Tab, so the assertion is that the handler declined
		// to act: a trap that fired on every press would jump back to the first control
		// and make the middle of a form unreachable.
		const middle = screen.getByRole('button', { name: 'Cancel' });
		middle.focus();
		await pressTab();

		expect(document.activeElement).toBe(middle);
	});

	it('does not treat Escape as a decision while it is closed', async () => {
		render(ModalHarness, { open: false });

		await userEvent.keyboard('{Escape}');

		// The page behind it is not a dialog, so a stray Escape must not consume a key
		// the operator meant for something else.
		expect(screen.getByText('page behind the dialog')).toBeInTheDocument();
	});

	it('sizes itself for what it holds, and centres by default', async () => {
		const { rerender } = render(ModalHarness, { open: true });
		const panel = screen.getByRole('dialog');

		// A short form and a document preview need different widths, and `md` is the
		// form's default rather than something every caller has to remember.
		expect(panel.className).toContain('max-w-md');
		expect(panel.className).toContain('max-h-[90vh]');
		expect(panel.parentElement?.className).toContain('items-center');
		expect(panel.parentElement?.className).toContain('justify-center');

		await rerender({ open: true, size: 'lg' });
		expect(screen.getByRole('dialog').className).toContain('max-w-3xl');
	});

	it('scrolls the body and holds the footer still, so the buttons stay reachable', async () => {
		render(ModalHarness, { open: true });

		const dialog = screen.getByRole('dialog');
		// A comb file is taller than any viewport, and a footer that scrolled away with
		// it would leave an operator with no way to close the preview.
		expect(dialog.querySelector('.card-body')?.className).toContain('overflow-y-auto');
		expect(dialog.querySelector('.card-actions')?.className).toContain('shrink-0');
	});

	it('draws no footer bar when the caller passed no footer', async () => {
		render(ModalHarness, { open: true, withFooter: false });

		expect(screen.queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
		expect(screen.getByRole('dialog').querySelector('.card-actions')).toBeNull();
	});
});
