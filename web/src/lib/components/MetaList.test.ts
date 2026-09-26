import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import MetaList from './MetaList.svelte';
import type { MetaRow } from '$lib/format';

const rows: MetaRow[] = [
	{ label: 'Trace', value: 'trace-01a0', mono: true },
	{ label: 'Branch', value: 'paseka/trace-01a0' },
	{ label: 'Pull request', value: 'https://forge.example.com/pull/42', href: 'https://forge.example.com/pull/42' }
];

describe('MetaList', () => {
	it('pairs every label with its value', () => {
		render(MetaList, { rows, label: 'Trail' });

		const list = screen.getByLabelText('Trail');
		expect(list.tagName).toBe('DL');
		expect(screen.getByText('Trace')).toBeInTheDocument();
		expect(screen.getByText('trace-01a0')).toBeInTheDocument();
		expect(screen.getByText('Branch')).toBeInTheDocument();
		expect(screen.getByText('paseka/trace-01a0')).toBeInTheDocument();
	});

	it('renders a monospace value only when the row asks for one', () => {
		render(MetaList, { rows, label: 'Trail' });
		expect(screen.getByText('trace-01a0').className).toContain('font-mono');
		expect(screen.getByText('paseka/trace-01a0').className).not.toContain('font-mono');
	});

	it('opens a linked value in a new tab without leaking the opener', () => {
		render(MetaList, { rows, label: 'Trail' });
		const link = screen.getByRole('link', { name: 'https://forge.example.com/pull/42' });
		expect(link).toHaveAttribute('target', '_blank');
		expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'));
	});

	it('keeps the full text of a truncating line in a hint on body', () => {
		render(MetaList, {
			rows: [{ label: 'Path', value: '/home/x/.paseka/w', mono: true, hint: ['/home/x/.paseka/w'] }],
			label: 'Worktree'
		});
		const hint = document.body.querySelector('div[aria-hidden="true"][id^="paseka-hint-"]');
		expect(hint).toHaveTextContent('/home/x/.paseka/w');
	});

	it('adds no hint for a row that does not truncate', () => {
		render(MetaList, { rows: [{ label: 'Runs', value: '4' }], label: 'Trail' });
		expect(document.body.querySelector('div[aria-hidden="true"][id^="paseka-hint-"]')).toBeNull();
	});

	it('lays out in one column on a phone and two above 768px', () => {
		const { container } = render(MetaList, { rows, label: 'Trail' });
		expect(container.querySelector('dl')?.className).toContain('sm:grid-cols-2');

		const single = render(MetaList, { rows, label: 'Trail', columns: 1 });
		expect(single.container.querySelector('dl')?.className).not.toContain('sm:grid-cols-2');
	});
});

describe('MetaList copy button', () => {
	function stubClipboard(): ReturnType<typeof vi.fn> {
		const writeText = vi.fn(async () => {});
		Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
		return writeText;
	}

	afterEach(() => {
		Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
		vi.restoreAllMocks();
	});

	it('offers a copy button only on a row that asked for one', () => {
		stubClipboard();
		render(MetaList, {
			rows: [
				{ label: 'Trace', value: 'trace-01a0', mono: true, copy: true },
				{ label: 'Runs', value: '4' }
			],
			label: 'Trail'
		});

		const button = screen.getByRole('button', { name: 'Copy trace' });
		expect(button).toHaveAttribute('title', 'Copy');
		expect(button.querySelector('svg')?.getAttribute('class')).toContain('lucide-copy');
		expect(screen.getAllByRole('button')).toHaveLength(1);
	});

	it('puts the row value on the clipboard and confirms it', async () => {
		const user = userEvent.setup();
		const writeText = stubClipboard();
		render(MetaList, {
			rows: [{ label: 'Trace', value: 'trace-01a0', mono: true, copy: true }],
			label: 'Trail'
		});

		await user.click(screen.getByRole('button', { name: 'Copy trace' }));

		expect(writeText).toHaveBeenCalledWith('trace-01a0');
		const confirmed = await screen.findByRole('button', { name: 'Trace copied' });
		expect(confirmed).toHaveAttribute('title', 'Copied');
		// An SVG `className` is an SVGAnimatedString, so read the attribute.
		expect(confirmed.querySelector('svg')?.getAttribute('class')).toContain('text-success');
		expect(confirmed.querySelector('svg')?.getAttribute('class')).toContain('lucide-check');
	});

	it('stays silent when the copy was refused', async () => {
		const user = userEvent.setup();
		Object.defineProperty(navigator, 'clipboard', {
			configurable: true,
			value: {
				writeText: async () => {
					throw new Error('denied');
				}
			}
		});
		const execCommand = vi.fn(() => false);
		Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand });
		render(MetaList, {
			rows: [{ label: 'Trace', value: 'trace-01a0', copy: true }],
			label: 'Trail'
		});

		await user.click(screen.getByRole('button', { name: 'Copy trace' }));

		await waitFor(() => expect(execCommand).toHaveBeenCalledWith('copy'));
		expect(screen.getByRole('button', { name: 'Copy trace' })).toHaveAttribute('title', 'Copy');
	});

	it('reverts the confirmation on its own', async () => {
		vi.useFakeTimers();
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		stubClipboard();
		render(MetaList, {
			rows: [{ label: 'Trace', value: 'trace-01a0', copy: true }],
			label: 'Trail'
		});

		await user.click(screen.getByRole('button', { name: 'Copy trace' }));
		await vi.advanceTimersByTimeAsync(2000);

		expect(screen.getByRole('button', { name: 'Copy trace' })).toHaveAttribute('title', 'Copy');
		vi.useRealTimers();
	});
});
