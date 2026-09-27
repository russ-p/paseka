import { render, screen, waitFor, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Settings from './+page.svelte';
import { createSettingsStore } from '$lib/stores/settings.svelte';
import { colonyConfig, configAdapters, configAdapter } from '../../tests/fixtures';
import type { ColonyConfig } from '$lib/api/types';

function harness(config: ColonyConfig = colonyConfig()) {
	const loadConfig = vi.fn(async () => config);
	const store = createSettingsStore({ loadConfig });
	return { store, loadConfig };
}

/** The `dl` a section renders, found by its accessible name. */
function metaList(name: string): HTMLElement {
	return screen.getByLabelText(name);
}

function rowValue(name: string, label: string): string {
	const row = within(metaList(name))
		.getAllByRole('term')
		.find((term) => term.textContent === label);
	if (!row) throw new Error(`no ${label} row in ${name}`);
	// The cell carries the copy button beside the value, so it is trimmed rather
	// than compared whole.
	return (row.nextElementSibling as HTMLElement).textContent?.trim() ?? '';
}

function headers(): string[] {
	return within(screen.getByRole('table')).getAllByRole('columnheader').map((cell) => cell.textContent ?? '');
}

/**
 * A cell by its row's first-column value and the column's name. The row is found
 * inside the table and by its first cell rather than by page text, because an
 * adapter's name and its binary are the same two letters often enough that
 * `getByText` alone is ambiguous.
 */
function cellNamed(row: string, column: string): HTMLElement {
	const index = headers().indexOf(column);
	if (index < 0) throw new Error(`no ${column} column in ${headers().join(', ')}`);
	const body = within(screen.getByRole('table')).getAllByRole('rowgroup')[1];
	const match = within(body)
		.getAllByRole('row')
		.find((candidate) => within(candidate).getAllByRole('cell')[0]?.textContent?.trim() === row);
	if (!match) throw new Error(`no ${row} row in the adapter table`);
	return within(match).getAllByRole('cell')[index] as HTMLElement;
}

describe('settings route', () => {
	it('reads the config once and never starts a timer', async () => {
		const { store, loadConfig } = harness();
		render(Settings, { store });

		await waitFor(() => expect(screen.getByLabelText('NATS transport')).toBeInTheDocument());
		// Config changes when an operator edits a file, so a poll would re-read
		// identical values forever. Refresh is the control that re-reads.
		expect(loadConfig).toHaveBeenCalledTimes(1);
	});

	it('names the source of a value rather than showing the value alone', async () => {
		const { store } = harness();
		render(Settings, { store });

		await waitFor(() => expect(metaList('NATS transport')).toBeInTheDocument());
		// `config.yaml` is what an operator editing the file expects, so it gets no
		// annotation; the default prefix is the surprising half and says so.
		expect(rowValue('NATS transport', 'URL')).toBe('nats://127.0.0.1:4222');
		expect(rowValue('NATS transport', 'Subject prefix')).toContain('the default');
	});

	it('warns that an env var outranks the file, and says so on the row too', async () => {
		const { store } = harness(
			colonyConfig({ nats: { url: { value: 'nats://elsewhere:4222', source: 'env:PASEKA_NATS_URL' }, subjectPrefix: { value: 'paseka.paseka', source: 'default' } } })
		);
		render(Settings, { store });

		await waitFor(() => expect(screen.getByRole('status')).toBeInTheDocument());
		// The operator is about to edit config.yaml, and without this the page would
		// have shown a value the process does not use and said nothing about it.
		expect(screen.getByRole('status')).toHaveTextContent('PASEKA_NATS_URL');
		expect(rowValue('NATS transport', 'URL')).toContain('$PASEKA_NATS_URL');
		expect(screen.getByText(/NATS is configured by \$PASEKA_NATS_URL/)).toBeInTheDocument();
	});

	it('says NATS is unconfigured rather than showing a blank URL', async () => {
		const { store } = harness(
			colonyConfig({ nats: { url: { value: '', source: 'unset' }, subjectPrefix: { value: 'paseka.paseka', source: 'default' } } })
		);
		render(Settings, { store });

		await waitFor(() => expect(screen.getByText(/NATS is not configured/)).toBeInTheDocument());
		expect(rowValue('NATS transport', 'URL')).toBe('not set');
		// An unset URL is not overridden, so the warning stays out of the way.
		expect(screen.queryByRole('status')).not.toBeInTheDocument();
	});

	it('shows the variable an adapter reads and never the key', async () => {
		const { store } = harness();
		render(Settings, { store });

		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());
		expect(cellNamed('cursor', 'API key env')).toHaveTextContent('CURSOR_API_KEY');
		// A declared name is not a key: only a resolving variable is news, and the
		// declared pi one is the row that carries it.
		expect(cellNamed('cursor', 'Key')).toHaveTextContent('not set');
		expect(cellNamed('pi', 'Key')).toHaveTextContent('set');
	});

	it('tells a loader default from a value someone wrote', async () => {
		const { store } = harness();
		render(Settings, { store });

		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());
		// The whole reason the endpoint sends a source: `agent` and `pi` are both
		// on screen, and one of them nobody wrote.
		expect(cellNamed('cursor', 'Binary')).toHaveTextContent('agent (the default)');
		expect(cellNamed('pi', 'Binary')).toHaveTextContent('pi');
		expect(cellNamed('pi', 'Binary')).not.toHaveTextContent('default');
		expect(cellNamed('pi', 'File')).toHaveTextContent('present');
		expect(cellNamed('cursor', 'File')).toHaveTextContent('inferred');
	});

	it('reports an adapter with no credential as having none', async () => {
		const { store } = harness(colonyConfig({ adapters: [configAdapter({ name: 'opencode', apiKeyEnv: { value: '', source: 'unset' } })] }));
		render(Settings, { store });

		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());
		expect(cellNamed('opencode', 'API key env')).toHaveTextContent('—');
		// A declared `badge` column that gets nothing says so, rather than falling
		// back to text and implying there is a variable that is merely unset.
		expect(cellNamed('opencode', 'Key')).toHaveTextContent('');
	});

	it('lists the seven push categories, with off as a mode rather than silence', async () => {
		const { store } = harness();
		render(Settings, { store });

		await waitFor(() => expect(screen.getByText('commit_gate')).toBeInTheDocument());
		for (const category of ['invites', 'blocked', 'failed', 'review_required', 'review_final', 'commit_gate', 'completed']) {
			expect(screen.getByText(category)).toBeInTheDocument();
		}
		// The legacy `waiting_review` is folded into the two review modes server-side,
		// so the page must not offer it as a category of its own.
		expect(screen.queryByText('waiting_review')).not.toBeInTheDocument();
		// An off category is a fact, not an absence — the badge says it.
		expect(screen.getByText('commit_gate').closest('li')).toHaveTextContent('off');
	});

	it('explains a colony with no gate rather than reporting a failure', async () => {
		const { store } = harness(
			colonyConfig({
				telegram: {
					present: false,
					enabled: false,
					mode: { value: '', source: 'unset' },
					botTokenSet: false,
					botTokenEnv: 'PASEKA_TELEGRAM_BOT_TOKEN',
					allowFrom: [],
					chatIds: [],
					notify: []
				}
			})
		);
		render(Settings, { store });

		await waitFor(() => expect(screen.getByText(/No telegram.yaml/)).toBeInTheDocument());
		// `telegram.Load` treats a missing file as an error; a settings page that
		// inherited that would render "not configured" as a broken read.
		expect(screen.queryByRole('alert')).not.toBeInTheDocument();
		expect(screen.queryByText('invites')).not.toBeInTheDocument();
	});

	it('names a gate that is present but switched off', async () => {
		const { store } = harness(
			colonyConfig({
				telegram: {
					present: true,
					enabled: false,
					mode: { value: 'longpoll', source: 'config.yaml' },
					botTokenSet: true,
					botTokenEnv: 'PASEKA_TELEGRAM_BOT_TOKEN',
					allowFrom: [],
					chatIds: [],
					notify: []
				}
			})
		);
		render(Settings, { store });

		await waitFor(() => expect(screen.getByText(/configured and switched off/)).toBeInTheDocument());
		expect(rowValue('Telegram gate', 'Enabled')).toBe('no');
		// The sentence is body copy, not a `Section` note: a note is set in caps, and
		// a whole sentence in caps is a different kind of shout.
		expect(screen.getByText(/configured and switched off/).tagName).toBe('P');
	});

	it('carries the theme picker, the one setting the console keeps itself', async () => {
		const { store } = harness();
		render(Settings, { store });

		// The picker is a fieldset with a legend rather than a labelled select, so
		// the combobox is reached by role and the legend asserted beside it.
		await waitFor(() => expect(screen.getByRole('combobox')).toBeInTheDocument());
		expect(screen.getByText('Theme')).toBeInTheDocument();
	});

	it('says the view is read-only, so no control here promises an edit', async () => {
		const { store } = harness();
		render(Settings, { store });

		await waitFor(() => expect(screen.getByText(/This view is read-only/)).toBeInTheDocument());
		// Only the theme select takes input, and the table's filter box narrows
		// what is already on screen. A text field or a Save button here would be a
		// promise the write endpoints cannot keep yet, and the page copy says so
		// rather than letting a control imply otherwise.
		expect(document.querySelectorAll('input:not([type="search"]), textarea')).toHaveLength(0);
		expect(screen.getAllByRole('combobox')).toHaveLength(1);
		const buttons = screen
			.getAllByRole('button')
			.map((b) => b.getAttribute('aria-label') ?? b.textContent?.trim())
			.filter((label) => label !== '');
		expect(buttons).toEqual(['Refresh', 'Copy slug', 'Copy root']);
	});

	it('re-reads on Refresh', async () => {
		const { store, loadConfig } = harness();
		render(Settings, { store });

		await waitFor(() => expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument());
		await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));

		await waitFor(() => expect(loadConfig).toHaveBeenCalledTimes(2));
	});

	it('filters the adapter table across columns', async () => {
		const { store } = harness();
		render(Settings, { store });

		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());
		await userEvent.type(screen.getByLabelText('Filter adapters'), 'GEMINI');

		// The variable name is searchable without being shown twice, and the join
		// into one cell is what makes that free.
		await waitFor(() => expect(cellNamed('pi', 'Adapter')).toBeInTheDocument());
		expect(screen.queryByText('cursor')).not.toBeInTheDocument();
	});

	it('skeletons before the first payload, then an alert on a failure', async () => {
		const { store } = harness();
		render(Settings, { store });

		expect(document.querySelectorAll('.skeleton').length).toBeGreaterThan(0);
		await waitFor(() => expect(screen.getByLabelText('NATS transport')).toBeInTheDocument());
		expect(document.querySelectorAll('.skeleton')).toHaveLength(0);
	});

	it('reports a failed read as an alert', async () => {
		const store = createSettingsStore({ loadConfig: vi.fn(async () => Promise.reject(new Error('config: permission denied'))) });
		render(Settings, { store });

		await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('config: permission denied'));
	});

	it('explains a colony with no adapters rather than an empty table', async () => {
		const { store } = harness(colonyConfig({ adapters: [] }));
		render(Settings, { store });

		await waitFor(() => expect(screen.getByText('No adapters are configured for this colony.')).toBeInTheDocument());
	});

	it('shows every adapter a colony carries, not only the launchable ones', async () => {
		const { store } = harness(colonyConfig({ adapters: configAdapters() }));
		render(Settings, { store });

		await waitFor(() => expect(screen.getByRole('table')).toBeInTheDocument());
		for (const name of ['cursor', 'pi', 'claude', 'opencode']) {
			expect(cellNamed(name, 'Adapter')).toBeInTheDocument();
		}
	});
});
