import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import StatusBadge from './StatusBadge.svelte';
import { statusTone, statusToneClasses } from '$lib/status';

describe('StatusBadge', () => {
	it('shows the status itself when no label is given, so the word and the tone cannot disagree', () => {
		render(StatusBadge, { status: 'waiting_review' });

		const badge = screen.getByText('waiting_review');
		expect(badge).toHaveTextContent('waiting_review');
		expect(badge).toHaveClass('badge', statusToneClasses.warning);
	});

	it('shows a label that names the decision rather than the state', () => {
		render(StatusBadge, { status: 'waiting_review', label: 'final gate' });

		// A row that says only `waiting_review` cannot say *which* gate, and the tone is
		// the same for every review — so the word beside it has to carry the difference.
		expect(screen.getByText('final gate')).toBeInTheDocument();
		expect(screen.queryByText('waiting_review')).not.toBeInTheDocument();
	});

	it('takes its tone from the shared table, so no page invents a palette beside the rest', () => {
		const cases: [string, string][] = [
			['running', 'badge-info'],
			['completed', 'badge-success'],
			['waiting_review', 'badge-warning'],
			['failed', 'badge-error'],
			['stopped', 'badge-neutral'],
			['sound', 'badge-warning'],
			['silent', 'badge-info'],
			['off', 'badge-neutral']
		];
		for (const [status, tone] of cases) {
			const view = render(StatusBadge, { status });
			expect(view.container.querySelector('.badge')?.className).toContain(tone);
			view.unmount();
		}
	});

	it('reads a status it has never seen as idle rather than as a fault', () => {
		// A new status from the server is not an error to shout about; it is a value
		// this console build has no colour for.
		render(StatusBadge, { status: 'quiescing' });

		const badge = screen.getByText('quiescing');
		expect(badge).toHaveClass('badge', 'badge-neutral');
		expect(statusTone('quiescing')).toBe('neutral');
	});

	it('is indifferent to case and surrounding space, because a status is a wire value', () => {
		const { container } = render(StatusBadge, { status: '  WAITING_REVIEW ' });

		// The word is shown exactly as the server sent it — a badge that rewrote the
		// status would be showing something the operator cannot grep for — while the
		// tone is looked up on the trimmed, lowercased value.
		const badge = container.querySelector('.badge');
		expect(badge).toHaveTextContent('WAITING_REVIEW');
		expect(badge).toHaveClass('badge-warning');
	});

	it('is a badge and not a control, so nothing on it looks pressable', () => {
		render(StatusBadge, { status: 'running' });

		const badge = screen.getByText('running');
		expect(badge.tagName).toBe('SPAN');
		// A status is read, not acted on; a badge that looked like a button would
		// promise an action the console has none for.
		expect(badge.className).not.toContain('btn');
		expect(badge).not.toHaveAttribute('tabindex');
	});
});
