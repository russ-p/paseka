import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import CodeBlock from './CodeBlock.svelte';

const code = JSON.stringify({ traceId: 'trace-01', seq: 12, dryRun: false, note: null }, null, 2);

/** The class on the one span wrapping this exact text, or `undefined` when it carries none. */
const classOn = (container: HTMLElement, text: string): string | undefined =>
	[...container.querySelectorAll('span')].find((span) => span.textContent === text)?.className;

describe('CodeBlock', () => {
	it('shows exactly the text it was given, with nothing of the template mixed in', () => {
		// The whole reason the `each` in the component is one line: Svelte keeps whitespace
		// inside a `<pre>`, so a newline between the tags would land in the JSON as data.
		// Compared as `textContent` because `toHaveTextContent` collapses whitespace, which
		// is precisely the difference this block cannot have.
		const { container } = render(CodeBlock, { code });
		expect(container.querySelector('pre')?.textContent).toBe(code);
	});

	it('tells a field name from its value, and a number from a keyword', () => {
		const { container } = render(CodeBlock, { code });
		expect(classOn(container, '"traceId"')).toBe('text-primary');
		expect(classOn(container, '"trace-01"')).toBe('text-success');
		expect(classOn(container, '12')).toBe('text-warning');
		expect(classOn(container, 'false')).toBe('text-secondary');
		expect(classOn(container, 'null')).toBe('text-secondary');
	});

	it('quiets the punctuation rather than colouring it as content', () => {
		const { container } = render(CodeBlock, { code: '{"a":1}' });
		expect(classOn(container, '{')).toBe('text-base-content/70');
		expect(classOn(container, ':')).toBe('text-base-content/70');
		expect(classOn(container, '}')).toBe('text-base-content/70');
	});

	it('leaves whitespace unclassed so it inherits the block instead of adding nodes of its own', () => {
		const { container } = render(CodeBlock, { code });
		const blank = [...container.querySelectorAll('span')].find((span) => span.textContent === '\n  ');
		expect(blank).toBeDefined();
		expect(blank?.hasAttribute('class')).toBe(false);
	});

	it('renders an author-supplied payload as text rather than as markup', () => {
		// `payload` is whatever the publishing bee put in it, so the block cannot be the
		// place a summary turns into an element. The quotes arrive backslash-escaped
		// because the text on screen is the envelope, not a re-parse of it.
		const { container } = render(CodeBlock, {
			code: JSON.stringify({ summary: '<img src=x onerror="alert(1)">' })
		});
		expect(container.querySelector('img')).toBeNull();
		expect(container.querySelector('pre')?.textContent).toContain('<img src=x onerror=');
	});

	it('renders input that is not JSON as itself, because the raw view must never hide a payload', () => {
		const { container } = render(CodeBlock, { code: 'not json at all' });
		expect(container.querySelector('pre')?.textContent).toBe('not json at all');
		// Unclassified words still get a span each, but never a colour: a highlighter that
		// guessed at prose would be making a claim about a payload it does not understand.
		[...container.querySelectorAll('span')].forEach((span) => {
			expect(span.hasAttribute('class')).toBe(false);
		});
	});

	it('names the block for a reader, since a pre of punctuation does not', () => {
		render(CodeBlock, { code, label: 'Raw event JSON' });
		expect(screen.getByLabelText('Raw event JSON').tagName).toBe('PRE');
	});

	it('takes an extra class so a folded view can cap its own height', () => {
		const { container } = render(CodeBlock, { code, class: 'mt-1 max-h-80' });
		expect(container.querySelector('pre')?.className).toContain('mt-1 max-h-80');
	});

	it('renders nothing rather than failing when the payload stringifies to nothing', () => {
		const { container } = render(CodeBlock, { code: '' });
		expect(container.querySelector('pre')?.textContent).toBe('');
	});
});
