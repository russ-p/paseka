import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import DiffViewer from './DiffViewer.svelte';
import { mergeDiff } from '../../tests/fixtures';
import type { MergeDiff } from '$lib/api/types';

/** Two files, so the list, the filter, and the per-file framing all have something to do. */
function twoFiles(overrides: Partial<MergeDiff> = {}): MergeDiff {
	return mergeDiff({
		stat: [
			' web/src/lib/diff.ts                | 3 +++',
			' internal/gate/telegram/config.go  | 3 ++-',
			' 2 files changed, 4 insertions(+), 1 deletion(-)',
			''
		].join('\n'),
		diff: [
			'diff --git a/web/src/lib/diff.ts b/web/src/lib/diff.ts',
			'index 1111111..2222222 100644',
			'--- a/web/src/lib/diff.ts',
			'+++ b/web/src/lib/diff.ts',
			'@@ -1,2 +1,3 @@',
			' export const first = 1;',
			'-export const second = 2;',
			'+export const third = 3;',
			'diff --git a/internal/gate/telegram/config.go b/internal/gate/telegram/config.go',
			'index 3333333..4444444 100644',
			'--- a/internal/gate/telegram/config.go',
			'+++ b/internal/gate/telegram/config.go',
			'@@ -1,2 +1,2 @@',
			' package telegram',
			'-const mode = "legacy"',
			'+const mode = "sound"',
			''
		].join('\n'),
		...overrides
	});
}

const renamed = mergeDiff({
	stat: ' internal/console/telemetry.go | 1 +\n 1 file changed\n',
	diff: [
		'diff --git a/internal/console/trace.go b/internal/console/telemetry.go',
		'similarity index 94%',
		'rename from internal/console/trace.go',
		'rename to internal/console/telemetry.go',
		'--- a/internal/console/trace.go',
		'+++ b/internal/console/telemetry.go',
		'@@ -1 +1,2 @@',
		' package console',
		'+const telemetry = true',
		''
	].join('\n')
});

const withBinary = mergeDiff({
	// A real stat, because the `· bin` mark is a suffix on the stat label: a file
	// git recorded as unchanged carries no label and therefore no mark.
	stat: ' web/static/logo.png | 2 +-\n 1 file changed\n',
	diff: [
		'diff --git a/web/static/logo.png b/web/static/logo.png',
		'index 5555555..6666666 100644',
		'Binary files a/web/static/logo.png and b/web/static/logo.png differ',
		''
	].join('\n')
});

beforeEach(() => {
	// jsdom has no layout, so `scrollIntoView` is not a function at all. The viewer
	// calls it to make a list click move the body, which is the list's whole job.
	Element.prototype.scrollIntoView = vi.fn();
});

describe('DiffViewer', () => {
	describe('when there is nothing to show', () => {
		it('says the worktree is gone rather than rendering an empty viewer', () => {
			render(DiffViewer, { diff: mergeDiff({ missingWorktree: true, diff: undefined }) });

			expect(screen.getByText(/No branch for this trail on this machine/)).toBeInTheDocument();
			// The whole story is that notice, so a filter box over an empty frame would
			// be offering to narrow nothing.
			expect(screen.queryByLabelText('Merge diff')).not.toBeInTheDocument();
			expect(screen.queryByLabelText('Changed files')).not.toBeInTheDocument();
		});

		it('names the two branches when there is no change between them', () => {
			render(DiffViewer, { diff: mergeDiff({ empty: true, diff: undefined }) });

			const notice = screen.getByText(/Nothing to merge/);
			expect(notice).toHaveTextContent('main');
			expect(notice).toHaveTextContent('paseka/trace-01a0bd6963faa14f');
			expect(screen.queryByLabelText('Merge diff')).not.toBeInTheDocument();
		});

		it('treats a patch that arrived empty as the same thing', () => {
			// The server sends an absent `diff` and an empty string for two different
			// reasons and they mean the same thing to a reviewer, so they read alike.
			render(DiffViewer, { diff: mergeDiff({ diff: '' }) });

			expect(screen.getByText(/Nothing to merge/)).toBeInTheDocument();
			expect(screen.queryByLabelText('Merge diff')).not.toBeInTheDocument();
		});
	});

	describe('warnings', () => {
		it('warns that the local branch is behind origin, and still shows the diff', () => {
			render(DiffViewer, { diff: mergeDiff({ originBehindCount: 3 }) });

			expect(screen.getByRole('alert')).toHaveTextContent('is 3 commit(s) behind origin');
			// A warning is not a veto: the reviewer is about to merge this exact patch,
			// which is the case the warning exists for.
			expect(screen.getByLabelText('Merge diff')).toBeInTheDocument();
		});

		it('says a truncated patch is incomplete rather than rendering it as whole', () => {
			render(DiffViewer, { diff: mergeDiff({ truncated: true }) });

			expect(screen.getByRole('alert')).toHaveTextContent('Diff truncated at the server’s size cap');
			// And the last file says so too, because the cut lands inside it rather
			// than between files.
			expect(within(screen.getByLabelText('Changed files')).getByText(/· trunc/)).toBeInTheDocument();
			expect(screen.getByText('Diff truncated — open the worktree locally for the rest of this file.')).toBeInTheDocument();
		});

		it('says nothing about a branch that is level with origin', () => {
			render(DiffViewer, { diff: twoFiles() });

			expect(screen.queryByRole('alert')).not.toBeInTheDocument();
		});
	});

	describe('the file list', () => {
		it('gives every file its path and its stat, wrapped rather than truncated', () => {
			render(DiffViewer, { diff: twoFiles() });

			const nav = screen.getByLabelText('Changed files');
			expect(within(nav).getByText('web/src/lib/diff.ts')).toBeInTheDocument();
			expect(within(nav).getByText('internal/gate/telegram/config.go')).toBeInTheDocument();
			expect(within(nav).getByText('+3 -0')).toBeInTheDocument();
			expect(within(nav).getByText('+2 -1')).toBeInTheDocument();
			// `config.go` and `config_test.go` truncated to the same prefix is the one
			// thing a diff file list must not do, so the path wraps instead.
			expect(within(nav).getByText('web/src/lib/diff.ts').className).toContain('break-all');
		});

		it('marks a binary file instead of pretending it has lines', () => {
			render(DiffViewer, { diff: withBinary });

			const nav = screen.getByLabelText('Changed files');
			expect(within(nav).getByText(/· bin/)).toBeInTheDocument();
			expect(screen.getByText('Binary file — there is no text diff to show.')).toBeInTheDocument();
			// A file with no text rows gets no table at all, rather than an empty one
			// that reads as "nothing changed in here".
			expect(screen.getByLabelText('web/static/logo.png').querySelector('table')).toBeNull();
		});

		it('shows both sides of a rename, because the old path is the one a reviewer knows', () => {
			render(DiffViewer, { diff: renamed });

			expect(screen.getByRole('heading', { name: /internal\/console\/telemetry\.go/ })).toHaveTextContent(
				'from internal/console/trace.go'
			);
		});

		it('navigates rather than filters: a click scrolls the body to that file', async () => {
			render(DiffViewer, { diff: twoFiles() });

			const nav = screen.getByLabelText('Changed files');
			await userEvent.click(within(nav).getByText('internal/gate/telegram/config.go'));

			expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: 'start' });
			expect(within(nav).getByText('internal/gate/telegram/config.go').closest('button')).toHaveAttribute(
				'aria-current',
				'true'
			);
		});

		it('says the filter matched nothing instead of an empty list with no explanation', async () => {
			render(DiffViewer, { diff: twoFiles() });

			await userEvent.type(screen.getByLabelText('Filter files by path'), 'zzz');

			expect(screen.getByText('No files match the filter.')).toBeInTheDocument();
		});

		it('keeps the body whole while the list filters, because a note anchors to a line number', async () => {
			render(DiffViewer, { diff: twoFiles() });

			await userEvent.type(screen.getByLabelText('Filter files by path'), 'telegram');

			const nav = screen.getByLabelText('Changed files');
			expect(within(nav).queryByText('web/src/lib/diff.ts')).not.toBeInTheDocument();
			// The hidden file's lines are still in the body, so every line number an
			// anchored note points at keeps meaning the same thing.
			const body = screen.getByLabelText('Merge diff');
			expect(within(body).getByText('export const third = 3;')).toBeInTheDocument();
			expect(within(body).getByText('const mode = "sound"')).toBeInTheDocument();
		});

		it('starts a new patch at its own first file rather than carrying a selection across', async () => {
			const { rerender } = render(DiffViewer, { diff: twoFiles() });

			const nav = screen.getByLabelText('Changed files');
			await userEvent.click(within(nav).getByText('internal/gate/telegram/config.go'));
			expect(within(nav).getByText('internal/gate/telegram/config.go').closest('button')).toHaveAttribute(
				'aria-current',
				'true'
			);

			rerender({ diff: mergeDiff() });

			// A selection is a path, so a patch that has never heard of it would leave
			// the body scrolled at a file the new trail does not have.
			expect(within(screen.getByLabelText('Changed files')).getByText('web/src/lib/diff.ts').closest('button')).toHaveAttribute(
				'aria-current',
				'true'
			);
		});
	});

	describe('the body', () => {
		it('renders lines with their own numbers rather than a raw patch', () => {
			render(DiffViewer, { diff: mergeDiff() });

			const body = screen.getByLabelText('Merge diff');
			// A hunk header is a row, which is what the parser decided; a raw `<pre>`
			// would show the same characters and a reviewer would have to read past them.
			expect(within(body).getByText('@@ -1,2 +1,3 @@')).toBeInTheDocument();
			expect(within(body).getByText('export const first = 1;')).toBeInTheDocument();
			// The addition is line 3 on the new side and has no old number at all.
			const added = within(body).getByText('export const third = 3;').closest('tr');
			expect(added?.querySelectorAll('td')[1]).toHaveTextContent('3');
			expect(added?.querySelectorAll('td')[0]).toHaveTextContent('');
		});

		it('renders a line as text, not as markup from the patch', () => {
			const { container } = render(DiffViewer, {
				diff: mergeDiff({
					diff: [
						'diff --git a/web/src/lib/x.ts b/web/src/lib/x.ts',
						'--- a/web/src/lib/x.ts',
						'+++ b/web/src/lib/x.ts',
						'@@ -0,0 +1 @@',
						'+const html = "<b>bold</b>";',
						''
					].join('\n')
				})
			});

			// The legacy handed diff2html the patch and scraped its generated DOM; a
			// file whose contents are HTML must arrive as characters, not as an element.
			expect(screen.getByText('const html = "<b>bold</b>";')).toBeInTheDocument();
			expect(container.querySelector('b')).toBeNull();
		});

		it('gives the unified layout a marker so a change is not carried by colour alone', () => {
			render(DiffViewer, { diff: twoFiles() });

			const markerOf = (line: string): string | undefined =>
				within(screen.getByLabelText('Merge diff'))
					.getByText(line)
					.closest('tr')
					?.querySelectorAll('td')[2]?.textContent;

			// A `+`, a `−`, and nothing at all, in their own column. A reader who cannot
			// see the tint has to be able to tell an addition from a removal.
			expect(markerOf('export const third = 3;')).toBe('+');
			expect(markerOf('export const second = 2;')).toBe('−');
			expect(markerOf('export const first = 1;')).toBe('');
		});

		it('says which half is which in split, and lets a note land on the old line of a context line', async () => {
			const onpickline = vi.fn();
			render(DiffViewer, { diff: mergeDiff(), onpickline });

			await userEvent.click(screen.getByRole('button', { name: 'Split' }));

			const body = screen.getByLabelText('Merge diff');
			// For anyone not reading the tints, and for a screen reader.
			expect(body.querySelector('caption')).toHaveTextContent('before on the left, after on the right');
			// An unchanged line is on both sides, and only the split view can aim a note
			// at the *old* one — which is usually the reason the note exists.
			const halves = within(body).getAllByText('export const first = 1;');
			expect(halves).toHaveLength(2);

			await userEvent.click(halves[0]);
			expect(onpickline).toHaveBeenLastCalledWith({
				path: 'web/src/lib/diff.ts',
				side: 'old',
				line: 1,
				snippet: 'export const first = 1;'
			});

			await userEvent.click(halves[1]);
			expect(onpickline).toHaveBeenLastCalledWith({
				path: 'web/src/lib/diff.ts',
				side: 'new',
				line: 1,
				snippet: 'export const first = 1;'
			});
		});

		it('pairs a removal with the addition that replaced it, on one row', async () => {
			const onpickline = vi.fn();
			render(DiffViewer, { diff: twoFiles(), onpickline });

			await userEvent.click(screen.getByRole('button', { name: 'Split' }));

			const file = screen.getByLabelText('web/src/lib/diff.ts');
			const pair = within(file).getByText('export const third = 3;').closest('tr');
			// The whole reason a side-by-side is trusted: the two sides cannot disagree
			// about a line number, because they are the same row.
			expect(within(pair as HTMLElement).getByText('export const second = 2;')).toBeInTheDocument();
			await userEvent.click(within(pair as HTMLElement).getByText('export const second = 2;'));
			expect(onpickline).toHaveBeenLastCalledWith({
				path: 'web/src/lib/diff.ts',
				side: 'old',
				line: 2,
				snippet: 'export const second = 2;'
			});
		});

		it('spans a hunk header across both halves rather than pairing it with a line', async () => {
			render(DiffViewer, { diff: mergeDiff() });
			await userEvent.click(screen.getByRole('button', { name: 'Split' }));

			const header = within(screen.getByLabelText('Merge diff')).getByText('@@ -1,2 +1,3 @@');
			expect(header.closest('td')).toHaveAttribute('colspan', '4');
		});

		it('draws nothing for a side a line does not exist on, and anchors nothing there', async () => {
			const onpickline = vi.fn();
			// Two removals replaced by one addition: the second row has no new side.
			render(DiffViewer, {
				diff: mergeDiff({
					diff: [
						'diff --git a/web/src/lib/diff.ts b/web/src/lib/diff.ts',
						'--- a/web/src/lib/diff.ts',
						'+++ b/web/src/lib/diff.ts',
						'@@ -1,3 +1 @@',
						'-const first = 1;',
						'-const second = 2;',
						'-const third = 3;',
						'+const all = 6;',
						''
					].join('\n')
				}),
				onpickline
			});
			await userEvent.click(screen.getByRole('button', { name: 'Split' }));

			const rows = within(screen.getByLabelText('web/src/lib/diff.ts')).getAllByRole('row');
			const fillerRow = rows.find((row) => row.querySelectorAll('td')[2]?.textContent === '');
			expect(fillerRow).toBeDefined();
			// A filler is not an empty numbered line, so nothing can be pointed at it.
			await userEvent.click(fillerRow as HTMLElement);
			expect(onpickline).not.toHaveBeenCalled();
		});

		it('refuses to anchor a row that is not a line of the file', async () => {
			const onpickline = vi.fn();
			render(DiffViewer, { diff: mergeDiff(), onpickline });

			await userEvent.click(within(screen.getByLabelText('Merge diff')).getByText('@@ -1,2 +1,3 @@'));
			await userEvent.click(within(screen.getByLabelText('Merge diff')).getByText('index 1111111..2222222 100644'));

			// A hunk header and a mode change are not lines anyone can leave a note on,
			// so the comments panel stays a plain consumer of this component.
			expect(onpickline).not.toHaveBeenCalled();
		});

		it('anchors an addition to the new side and a removal to the old', async () => {
			const onpickline = vi.fn();
			render(DiffViewer, { diff: twoFiles(), onpickline });

			const body = screen.getByLabelText('Merge diff');
			await userEvent.click(within(body).getByText('export const third = 3;'));
			expect(onpickline).toHaveBeenLastCalledWith({
				path: 'web/src/lib/diff.ts',
				side: 'new',
				// The second line of the new file: line 2 was replaced, not appended to.
				line: 2,
				snippet: 'export const third = 3;'
			});

			await userEvent.click(within(body).getByText('export const second = 2;'));
			expect(onpickline).toHaveBeenLastCalledWith({
				path: 'web/src/lib/diff.ts',
				side: 'old',
				line: 2,
				snippet: 'export const second = 2;'
			});
		});

		it('offers nothing to click when no panel is listening, rather than a cursor that lies', () => {
			render(DiffViewer, { diff: mergeDiff() });

			// The proposal page shows the same viewer with no comments panel beside it.
			// A dotted underline there would promise a note that cannot be written.
			const body = screen.getByLabelText('Merge diff');
			expect(within(body).getByText('export const third = 3;').closest('td')?.className).not.toContain(
				'cursor-pointer'
			);
		});
	});
});
