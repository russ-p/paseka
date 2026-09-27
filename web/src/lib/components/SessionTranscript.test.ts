import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import SessionTranscript from './SessionTranscript.svelte';
import { transcriptEntry } from '../../tests/fixtures';
import type { TranscriptEntry } from '$lib/api/types';

function lines(count: number): TranscriptEntry[] {
	return Array.from({ length: count }, (_, index) =>
		transcriptEntry({
			at: `2026-09-25T18:0${Math.min(index, 9)}:00Z`,
			role: index % 2 === 0 ? 'agent' : 'system',
			content: `line ${index + 1}`
		})
	);
}

function viewport(): HTMLElement {
	return screen.getByLabelText('Transcript lines');
}

/**
 * jsdom has no layout, so every scroll box reports zero. The tail-following contract is
 * a function of `scrollHeight`/`scrollTop`/`clientHeight`, so a test states those three
 * numbers itself and fires the scroll the component listens for.
 */
function scrollBox(
	element: HTMLElement,
	metrics: { scrollHeight: number; scrollTop: number; clientHeight: number }
): void {
	Object.defineProperty(element, 'scrollHeight', { configurable: true, value: metrics.scrollHeight });
	Object.defineProperty(element, 'scrollTop', {
		configurable: true,
		writable: true,
		value: metrics.scrollTop
	});
	Object.defineProperty(element, 'clientHeight', { configurable: true, value: metrics.clientHeight });
}

describe('SessionTranscript', () => {
	it('says what an empty transcript means rather than showing an empty box', () => {
		render(SessionTranscript, { lines: [] });

		// A session that wrote nothing is not a broken read, and the sentence says where
		// the real output is — the terminal, which this component is not.
		expect(screen.getByText(/Nothing was written to this session's transcript/)).toBeInTheDocument();
		expect(screen.getByText('transcript.ndjson')).toBeInTheDocument();
		expect(screen.queryByLabelText('Transcript lines')).not.toBeInTheDocument();
	});

	it('shows a skeleton-free box while the first read is in flight', () => {
		render(SessionTranscript, { lines: [], loading: true });

		// The empty state would claim nothing was written, which is a different claim
		// from "we have not asked yet".
		expect(screen.queryByText(/Nothing was written/)).not.toBeInTheDocument();
		expect(screen.getByText('Reading…')).toBeInTheDocument();
	});

	it('names who said each line, in words rather than by colour alone', () => {
		render(SessionTranscript, { lines: lines(2) });

		const rows = viewport().querySelectorAll('.flex.gap-3');
		expect(rows).toHaveLength(2);
		expect(rows[0]?.textContent).toContain('agent');
		expect(rows[0]?.textContent).toContain('line 1');
		expect(rows[1]?.textContent).toContain('system');
		// The tint is decoration; a reader who cannot see the colour still reads the role.
		expect(rows[0]?.querySelector('span')?.className).toContain('text-success');
		expect(rows[1]?.querySelector('span')?.className).toContain('text-info');
	});

	it('does not wrap a line, because this is a terminal', () => {
		const { container } = render(SessionTranscript, {
			lines: [transcriptEntry({ content: '  /help  →  1. continue  2. quit' })]
		});

		// The real session holds a two-column slash-command menu, and wrapping folds
		// every aligned column into a staircase. The double spaces are the alignment,
		// which is also why the text is asserted off the element rather than by query.
		const content = container.querySelector('.min-w-0.flex-1');
		expect(content?.textContent).toBe('  /help  →  1. continue  2. quit');
		expect(content?.className).not.toContain('whitespace-pre-wrap');
		expect(content?.className).toContain('flex-1');
	});

	it('is focusable, because a scrollable div is otherwise unreachable with the arrow keys', () => {
		render(SessionTranscript, { lines: lines(1) });

		const region = viewport();
		// The one WCAG 2.1.1 case the rule's authors did not model: a region that
		// scrolls, which is exactly what this one is for.
		expect(region).toHaveAttribute('tabindex', '0');
		expect(region).toHaveAttribute('role', 'region');
	});

	it('is a region and not a log, so reading history is not interrupted line by line', () => {
		render(SessionTranscript, { lines: lines(1) });

		// `log` implies `aria-live="polite"`, and announcing every line an agent writes
		// while someone is reading history is the opposite of helpful.
		expect(viewport()).not.toHaveAttribute('aria-live');
		expect(screen.getByLabelText('Transcript')).toBeInTheDocument();
	});

	it('says how much was dropped, because a tail is not the whole session', () => {
		render(SessionTranscript, { lines: lines(2), dropped: 1 });

		expect(screen.getByText(/1 earlier line is not shown/)).toBeInTheDocument();
	});

	it('counts the dropped lines in the plural', () => {
		render(SessionTranscript, { lines: lines(2), dropped: 42 });

		expect(screen.getByText(/42 earlier lines are not shown/)).toBeInTheDocument();
	});

	it('says nothing about lines it dropped when it dropped none', () => {
		render(SessionTranscript, { lines: lines(2), dropped: 0 });

		expect(screen.queryByText(/earlier line/)).not.toBeInTheDocument();
	});

	it('follows the newest line while the reader is already there', async () => {
		render(SessionTranscript, { lines: lines(3) });
		const region = viewport();
		scrollBox(region, { scrollHeight: 900, scrollTop: 700, clientHeight: 200 });

		await fireEvent.scroll(region);

		// The tail is where a reader watching a session finish wants to be.
		expect(region.scrollTop).toBe(900);
	});

	it('stops following the moment the reader reaches for history', async () => {
		render(SessionTranscript, { lines: lines(3) });
		const region = viewport();
		scrollBox(region, { scrollHeight: 900, scrollTop: 40, clientHeight: 200 });

		await fireEvent.scroll(region);

		// The legacy forced the tail on every tick, so the line a reader had just
		// reached for was taken away by the next poll.
		expect(screen.getByRole('button', { name: 'Latest' })).toBeInTheDocument();
	});

	it('stays put when a new page arrives while the reader is scrolled up', async () => {
		const view = render(SessionTranscript, { lines: lines(3) });
		const region = viewport();
		scrollBox(region, { scrollHeight: 900, scrollTop: 40, clientHeight: 200 });
		await fireEvent.scroll(region);

		region.scrollTop = 40;
		await view.rerender({ lines: lines(6) });

		await waitFor(() => expect(screen.getByText('line 6')).toBeInTheDocument());
		expect(region.scrollTop).toBe(40);
	});

	it('tolerates a reader a few pixels short of the bottom', async () => {
		render(SessionTranscript, { lines: lines(3) });
		const region = viewport();
		// Within a finger's width of the tail, which is where a reader scrolling with a
		// trackpad actually lands.
		scrollBox(region, { scrollHeight: 900, scrollTop: 690, clientHeight: 200 });

		await fireEvent.scroll(region);

		expect(screen.queryByRole('button', { name: 'Latest' })).not.toBeInTheDocument();
	});

	it('offers a way back to the tail, and takes the reader there', async () => {
		render(SessionTranscript, { lines: lines(3) });
		const region = viewport();
		scrollBox(region, { scrollHeight: 900, scrollTop: 40, clientHeight: 200 });
		await fireEvent.scroll(region);

		// Reading history is a mode the reader chose, so it gets a way back rather than
		// being yanked out of it by the next poll.
		await userEvent.click(screen.getByRole('button', { name: 'Latest' }));

		expect(region.scrollTop).toBe(900);
		expect(screen.queryByRole('button', { name: 'Latest' })).not.toBeInTheDocument();
	});

	it('reports a failed read without throwing away the lines it already has', () => {
		render(SessionTranscript, { lines: lines(2), error: 'transcript unreadable' });

		expect(screen.getByRole('alert')).toHaveTextContent('transcript unreadable');
		// A finished session's transcript is read on demand, so one bad page must not
		// empty what the reader was halfway through.
		expect(screen.getByText('line 1')).toBeInTheDocument();
	});

	it('bounds the window, because a long session would otherwise grow the page', () => {
		render(SessionTranscript, { lines: lines(1) });

		const region = viewport();
		expect(region.className).toContain('max-h-[32rem]');
		expect(region.className).toContain('overflow-auto');
	});
});
