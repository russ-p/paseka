import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import HintHarness from './HintHarness.svelte';

function tip(): HTMLElement {
	return document.body.querySelector('div[aria-hidden="true"][id^="paseka-hint-"]') as HTMLElement;
}

describe('Hint', () => {
	it('shows the full text of a value the row had to truncate', () => {
		render(HintHarness, { lines: ['nats://127.0.0.1:4222', 'Loaded from config.yaml.'] });

		// Portaled to `body` so a scrolling ancestor — the topbar row — cannot clip it,
		// which is also why the text is found there rather than in the component.
		expect(tip()).toHaveTextContent('nats://127.0.0.1:4222');
		expect(tip()).toHaveTextContent('Loaded from config.yaml.');
	});

	it('puts each line on its own, because a hint is read line by line', () => {
		render(HintHarness, { lines: ['One load 5m ago', 'Ahead by 3'] });

		const lines = tip().querySelectorAll('span');
		expect(lines).toHaveLength(2);
		expect(lines[0]).toHaveTextContent('One load 5m ago');
		expect(lines[1]).toHaveTextContent('Ahead by 3');
	});

	it('is invisible until the reader asks for it', async () => {
		render(HintHarness, { lines: ['Loaded from config.yaml.'] });

		// Always in the DOM and always holding the text, because a tooltip that had to
		// be fetched on hover would arrive after the reader had already looked away.
		expect(tip()).toHaveClass('opacity-0');
		expect(tip()).toHaveAttribute('aria-hidden', 'true');

		await userEvent.hover(screen.getByText('main'));

		expect(tip()).toHaveClass('opacity-100');
	});

	it('hides again when the pointer leaves', async () => {
		render(HintHarness, { lines: ['Loaded from config.yaml.'] });
		await userEvent.hover(screen.getByText('main'));
		expect(tip()).toHaveClass('opacity-100');

		await userEvent.unhover(screen.getByText('main'));

		expect(tip()).toHaveClass('opacity-0');
	});

	it('opens on focus, because a hint only on hover is a hint a keyboard never sees', async () => {
		render(HintHarness, { lines: ['Loaded from config.yaml.'] });

		await userEvent.tab();

		expect(document.activeElement).toBe(screen.getByText('main'));
		expect(tip()).toHaveClass('opacity-100');
	});

	it('closes on blur, so a hint does not hang over the next thing the reader looks at', async () => {
		render(HintHarness, { lines: ['Loaded from config.yaml.'] });
		await userEvent.tab();
		expect(tip()).toHaveClass('opacity-100');

		await userEvent.tab();

		expect(document.activeElement).toBe(screen.getByRole('button', { name: 'next control' }));
		expect(tip()).toHaveClass('opacity-0');
	});

	it('places itself next to the anchor rather than inside the row that scrolls', async () => {
		render(HintHarness, { lines: ['Loaded from config.yaml.'] });

		await userEvent.hover(screen.getByText('main'));

		// `position: fixed` with a computed top/left is the whole mechanism: in a
		// scrollable ancestor a popover would be clipped by the container that scrolls.
		expect(tip().className).toContain('fixed');
		expect(tip().getAttribute('style')).toMatch(/^top: \d+px; left: \d+px;$/);
	});

	it('stays on screen rather than hanging off the right edge', async () => {
		render(HintHarness, { lines: ['A very long explanation that will not fit beside the anchor.'] });
		const anchor = screen.getByText('main');
		// jsdom reports zeroes for every box, so the anchor is placed by hand: a popover
		// whose own width would push it past the viewport is a popover nobody reads.
		anchor.getBoundingClientRect = () =>
			({ left: 4000, right: 4100, top: 10, bottom: 30, width: 100, height: 20 }) as DOMRect;
		vi.stubGlobal('innerWidth', 1024);
		vi.stubGlobal('innerHeight', 768);
		tip().getBoundingClientRect = () => ({ width: 300, height: 40 }) as DOMRect;

		await userEvent.hover(anchor);

		const left = Number(/left: (\d+)px/.exec(tip().getAttribute('style') ?? '')?.[1]);
		expect(left).toBeLessThanOrEqual(1024 - 300 - 8);
		vi.unstubAllGlobals();
	});

	it('renders the content it wraps, unchanged', () => {
		const { container } = render(HintHarness, { lines: ['x'] });

		// The wrapper is a `<span>` with `min-w-0 flex-1` rather than `w-full`: beside
		// another control in a flex row, a full width claims the row and spills the
		// value back over its own label.
		const wrapper = container.querySelector('[data-hint]');
		expect(wrapper?.className).toContain('min-w-0');
		expect(wrapper?.className).toContain('flex-1');
		expect(wrapper?.className).not.toContain('w-full');
		expect(screen.getByText('main')).toBeInTheDocument();
	});

	it('gives two hints on one page two ids, so one cannot be mistaken for the other', () => {
		const first = render(HintHarness, { lines: ['first'] });
		const second = render(HintHarness, { lines: ['second'] });

		// `aria-hidden` means neither is announced, so the id is what tells the two
		// apart for anything scripted — and a shared one would make the second hint
		// appear on hover over the first.
		const ids = [...document.body.querySelectorAll('div[id^="paseka-hint-"]')].map(
			(node) => node.id
		);
		expect(new Set(ids).size).toBe(2);
		expect(first.container.querySelector('[data-hint]')?.getAttribute('data-hint')).not.toBe(
			second.container.querySelector('[data-hint]')?.getAttribute('data-hint')
		);
	});

	it('takes its tip out of the page when it unmounts', () => {
		const view = render(HintHarness, { lines: ['Loaded from config.yaml.'] });
		expect(tip()).not.toBeNull();

		view.unmount();

		// The tip is portaled to `body`, so Svelte's own teardown does not reach it
		// unless the action says so — a row left behind would follow the next hover.
		expect(tip()).toBeNull();
	});
});
