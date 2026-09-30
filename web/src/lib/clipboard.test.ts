import { afterEach, describe, expect, it, vi } from 'vitest';
import { copyText } from './clipboard';

function stubClipboard(writeText: (text: string) => Promise<void>): void {
	Object.defineProperty(navigator, 'clipboard', {
		configurable: true,
		value: { writeText }
	});
}

function clearClipboard(): void {
	Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
}

function stubExecCommand(result: boolean | (() => never)): ReturnType<typeof vi.fn> {
	const mock = vi.fn(typeof result === 'boolean' ? () => result : result);
	Object.defineProperty(document, 'execCommand', { configurable: true, value: mock });
	return mock;
}

afterEach(() => {
	clearClipboard();
	vi.restoreAllMocks();
});

describe('copyText', () => {
	it('uses the async clipboard when the page is a secure context', async () => {
		const writeText = vi.fn(async () => {});
		stubClipboard(writeText);

		expect(await copyText('trace-01a0')).toBe(true);
		expect(writeText).toHaveBeenCalledWith('trace-01a0');
	});

	it('falls back to a selection when there is no async clipboard', async () => {
		clearClipboard();
		const execCommand = stubExecCommand(true);

		expect(await copyText('trace-01a0')).toBe(true);
		expect(execCommand).toHaveBeenCalledWith('copy');
	});

	it('falls back when the async clipboard is refused', async () => {
		stubClipboard(async () => {
			throw new Error('denied');
		});
		const execCommand = stubExecCommand(true);

		expect(await copyText('trace-01a0')).toBe(true);
		expect(execCommand).toHaveBeenCalledWith('copy');
	});

	it('leaves no scratch field behind, whatever the outcome', async () => {
		clearClipboard();
		stubExecCommand(true);
		const before = document.querySelectorAll('textarea').length;

		await copyText('trace-01a0');

		expect(document.querySelectorAll('textarea').length).toBe(before);
	});

	it('reports a refusal instead of pretending it worked', async () => {
		clearClipboard();
		stubExecCommand(() => {
			throw new Error('not allowed');
		});

		expect(await copyText('trace-01a0')).toBe(false);
		expect(document.querySelectorAll('textarea').length).toBe(0);
	});
});
