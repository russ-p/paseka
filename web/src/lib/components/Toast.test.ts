import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import Toast from './Toast.svelte';
import { createToastStore } from '$lib/stores/toast.svelte';

describe('Toast', () => {
	it('keeps a polite live region on the page even with nothing to say', () => {
		render(Toast, { store: createToastStore(0) });

		// The region is permanent, because a live region inserted at the moment a toast
		// arrives is not announced by anything: the observer has to already be there.
		const region = screen.getByLabelText('Notifications');
		expect(region).toHaveAttribute('aria-live', 'polite');
		expect(region).toHaveAttribute('id', 'toast-region');
		expect(region.querySelectorAll('.alert')).toHaveLength(0);
	});

	it('says what happened, in the tone the caller chose', () => {
		const store = createToastStore(0);
		store.push('success', 'Merged into main.');
		render(Toast, { store });

		const toast = screen.getByText('Merged into main.');
		// The tone is the only thing carrying the difference between a success and a
		// failure, so it is a class the design system owns rather than a word.
		expect(toast.closest('.alert')).toHaveClass('alert-success');
	});

	it('tells a warning from an error, because a partial delete is not a failed one', () => {
		const store = createToastStore(0);
		store.push('warning', 'Deleted 2 of 3; paseka/trace-019 was refused.');
		store.push('error', 'origin unreachable');
		render(Toast, { store });

		expect(
			screen.getByText(/paseka\/trace-019 was refused/).closest('.alert')
		).toHaveClass('alert-warning');
		expect(screen.getByText('origin unreachable').closest('.alert')).toHaveClass('alert-error');
	});

	it('stacks several toasts without losing any', () => {
		const store = createToastStore(0);
		store.push('info', 'Committed to paseka/trace-1');
		store.push('success', 'Merged into main.');
		render(Toast, { store });

		const region = screen.getByLabelText('Notifications');
		expect(within(region).getByText('Committed to paseka/trace-1')).toBeInTheDocument();
		expect(within(region).getByText('Merged into main.')).toBeInTheDocument();
	});

	it('dismisses on the button, and only that one', async () => {
		const store = createToastStore(0);
		store.push('info', 'Committed to paseka/trace-1');
		store.push('error', 'origin unreachable');
		render(Toast, { store });

		const [first, second] = screen.getAllByRole('button', { name: 'Dismiss notification' });
		expect(first?.id).not.toBe(second?.id);
		await userEvent.click(first as HTMLElement);

		expect(screen.queryByText('Committed to paseka/trace-1')).not.toBeInTheDocument();
		// Dismissing one report is not a reason to discard the next.
		expect(screen.getByText('origin unreachable')).toBeInTheDocument();
	});

	it('goes away on its own after the timeout the store was given', async () => {
		const store = createToastStore(10);
		store.push('info', 'Fetch complete');
		render(Toast, { store });

		expect(screen.getByText('Fetch complete')).toBeInTheDocument();

		await waitFor(() => expect(screen.queryByText('Fetch complete')).not.toBeInTheDocument());
	});

	it('keeps a toast whose timeout is off, because a test store and a real one differ', () => {
		const store = createToastStore(0);
		store.push('info', 'Fetch complete');
		render(Toast, { store });

		// `0` is how a page with its own reporting opts out, and a store that ignored
		// it would empty the region a moment after every report.
		expect(screen.getByText('Fetch complete')).toBeInTheDocument();
	});

	it('parks itself at the bottom right, out of the way of the topbar', () => {
		render(Toast, { store: createToastStore(0) });

		const region = screen.getByLabelText('Notifications');
		expect(region).toHaveClass('toast-end');
		expect(region).toHaveClass('toast-bottom');
		// Above the dialogs and drawers it reports on, so a prune that opens neither
		// cannot cover the reason it failed.
		expect(region.className).toContain('z-50');
	});
});
