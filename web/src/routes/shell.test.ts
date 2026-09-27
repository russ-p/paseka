import { createRawSnippet } from 'svelte';
import { render } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { setShellPath } from '../tests/shell-page.svelte';
import Layout from './+layout.svelte';

const goto = vi.fn(async () => {});

vi.mock('$app/navigation', () => ({
	goto: (...args: unknown[]) => goto(...(args as [])),
	// The same stand-in `tests/setup.ts` installs, restated because a mock this narrow
	// replaces the module whole — and reaching for the module's own helper here would be
	// circular, since `list-state` imports the very module being mocked.
	replaceState: (url: string | URL, state: App.PageState) =>
		window.history.replaceState(state, '', url)
}));

vi.mock('$app/state', async () => {
	const { shellPage } = await import('../tests/shell-page.svelte');
	return { page: { get url() { return shellPage.url; } } };
});

const children = createRawSnippet(() => ({
	render: () => '<p>the page</p>'
}));

function at(path: string): void {
	setShellPath(path);
}

function renderShell(): { unmount: () => void } {
	return render(Layout, { props: { children } });
}

/** A `Modal` on screen, which is what makes the document say a dialog is open. */
function openDialog(): HTMLElement {
	const dialog = document.createElement('div');
	dialog.setAttribute('role', 'dialog');
	document.body.append(dialog);
	return dialog;
}

/** The list filter a route's `DataTable` renders. */
function listFilterOnScreen(): HTMLInputElement {
	const input = document.createElement('input');
	input.setAttribute('data-list-filter', '');
	input.value = 'scout';
	document.body.append(input);
	return input;
}

afterEach(() => {
	document.body.innerHTML = '';
	vi.clearAllMocks();
	at('/next/traces');
});

describe('shell keys', () => {
	it('takes Escape back to the list a detail belongs to', async () => {
		at('/next/traces/trace-01a0bd6963faa14f');
		renderShell();

		await userEvent.keyboard('{Escape}');

		expect(goto).toHaveBeenCalledWith('/next/traces');
	});

	it('leaves Escape alone on a list, because there is nowhere to go', async () => {
		at('/next/traces');
		renderShell();

		await userEvent.keyboard('{Escape}');

		expect(goto).not.toHaveBeenCalled();
	});

	it('yields Escape to a dialog, which closes itself and is the only thing that should', async () => {
		at('/next/traces/trace-01a0bd6963faa14f');
		renderShell();
		const dialog = openDialog();

		await userEvent.keyboard('{Escape}');

		// A second, invisible way to dismiss a dialog is how a form loses what was typed.
		expect(goto).not.toHaveBeenCalled();
		expect(dialog).toBeInTheDocument();
	});

	it('yields Escape to an editable target, so a half-written note is never thrown away', async () => {
		at('/next/reviews/trace-01a0bd6963faa14f/task-b2');
		renderShell();
		const note = document.createElement('textarea');
		document.body.append(note);
		note.focus();

		await userEvent.keyboard('{Escape}');

		expect(goto).not.toHaveBeenCalled();
	});

	it('yields Escape to a terminal, whose helper textarea is where the keystroke lands', async () => {
		// xterm focuses an off-screen textarea, so the editable rule already covers the
		// case the backlog named: the key never reaches the window to be claimed.
		at('/next/sessions/session-01');
		renderShell();
		const helper = document.createElement('textarea');
		document.body.append(helper);
		helper.focus();

		await userEvent.keyboard('{Escape}');

		expect(goto).not.toHaveBeenCalled();
	});

	it('focuses and selects the list filter on /', async () => {
		renderShell();
		const filter = listFilterOnScreen();

		await userEvent.keyboard('/');

		expect(document.activeElement).toBe(filter);
		// Arriving at a filter you meant to replace is the normal reason to press it.
		expect(filter.selectionStart).toBe(0);
		expect(filter.selectionEnd).toBe('scout'.length);
	});

	it('says nothing on / where there is no list, rather than swallowing the key', async () => {
		at('/next/topology');
		renderShell();

		await userEvent.keyboard('/');

		expect(goto).not.toHaveBeenCalled();
	});

	it('keeps the chord map working, because these two keys are not chords', async () => {
		at('/next/traces');
		renderShell();

		await userEvent.keyboard('gu');

		expect(goto).toHaveBeenCalledWith('/next/runs');
	});
});
