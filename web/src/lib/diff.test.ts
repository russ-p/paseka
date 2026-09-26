import { describe, expect, it } from 'vitest';
import {
	buildMergeDiffFiles,
	diffStatLabel,
	filterDiffFiles,
	parseDiffRows,
	parseMergeDiffStat,
	rowAnchor,
	splitMergeDiffPatch
} from './diff';

/** A real `git diff` of two files, one changed and one added. */
const patch = `diff --git a/internal/console/handlers.go b/internal/console/handlers.go
index 1f2a3b4..5c6d7e8 100644
--- a/internal/console/handlers.go
+++ b/internal/console/handlers.go
@@ -10,7 +10,8 @@ func (a *api) handleTasks(w http.ResponseWriter, r *http.Request) {
 	switch r.Method {
 	case http.MethodGet:
 		view, err := hiveview.ListTaskBoard(a.ctx)
-		writeError(w, err)
+		if err != nil {
+			writeError(w, err)
+		}
 		writeJSON(w, view)
 	}
 }
diff --git a/web/src/lib/diff.ts b/web/src/lib/diff.ts
new file mode 100644
index 0000000..a1b2c3d
--- /dev/null
+++ b/web/src/lib/diff.ts
@@ -0,0 +1,3 @@
+export const first = 1;
+export const second = 2;
+export const third = 3;
`;

const stat = ` internal/console/handlers.go | 5 +++--
 web/src/lib/diff.ts              | 3 +++
 2 files changed, 5 insertions(+), 3 deletions(-)
`;

describe('parseMergeDiffStat', () => {
	it('reads the added and removed split out of the bar, not out of the patch', () => {
		const counts = parseMergeDiffStat(stat);

		// The only place the split exists is the `+++--` bar, so a patch-only parser
		// could never produce these numbers.
		expect(counts['internal/console/handlers.go']).toEqual({ changes: 5, adds: 3, deletes: 2 });
		expect(counts['web/src/lib/diff.ts']).toEqual({ changes: 3, adds: 3, deletes: 0 });
	});

	it('ignores the summary line and returns nothing for no stat', () => {
		expect(parseMergeDiffStat(' 2 files changed, 5 insertions(+), 3 deletions(-)')).toEqual({});
		expect(parseMergeDiffStat(undefined)).toEqual({});
	});

	it('labels a file by its bar, and by its line count when the bar is empty', () => {
		expect(diffStatLabel({ changes: 12, adds: 4, deletes: 1 })).toBe('+4 -1');
		expect(diffStatLabel({ changes: 7, adds: 0, deletes: 0 })).toBe('7 lines');
		expect(diffStatLabel(undefined)).toBe('');
		expect(diffStatLabel({ changes: 0, adds: 0, deletes: 0 })).toBe('');
	});
});

describe('splitMergeDiffPatch', () => {
	it('splits on the diff header and takes the b-side path', () => {
		const files = splitMergeDiffPatch(patch, false);

		expect(files.map((file) => file.path)).toEqual([
			'internal/console/handlers.go',
			'web/src/lib/diff.ts'
		]);
	});

	it('keeps both sides of a rename, because the old path is the thing a reviewer knew', () => {
		const files = splitMergeDiffPatch(
			[
				'diff --git a/internal/old.go b/internal/new.go',
				'similarity index 95%',
				'rename from internal/old.go',
				'rename to internal/new.go',
				'--- a/internal/old.go',
				'+++ b/internal/new.go',
				'@@ -1,1 +1,1 @@',
				'-old',
				'+new',
				''
			].join('\n'),
			false
		);

		expect(files[0].path).toBe('internal/new.go');
		expect(files[0].oldPath).toBe('internal/old.go');
	});

	it('falls back to a positional name for a header it cannot read', () => {
		const files = splitMergeDiffPatch('diff --git nonsense\n@@ -1 +1 @@\n-a\n+b\n', false);

		expect(files[0].path).toBe('file-0');
	});

	it('finds a binary patch whether it is the first file or the last', () => {
		// The legacy test needed a leading newline for this to match, which meant a
		// binary file at the very start of a patch went unrecognised.
		const binaryFirst = splitMergeDiffPatch(
			'diff --git a/logo.png b/logo.png\nindex 111..222 100644\nBinary files a/logo.png and b/logo.png differ\n',
			false
		);
		expect(binaryFirst[0].binary).toBe(true);

		const binaryLast = splitMergeDiffPatch(
			`${patch}diff --git a/logo.png b/logo.png\nGIT binary patch\nliteral 12\n`,
			false
		);
		expect(binaryLast.at(-1)?.binary).toBe(true);
		expect(binaryLast[0].binary).toBe(false);
	});

	it('returns nothing for an empty or absent patch', () => {
		expect(splitMergeDiffPatch('', false)).toEqual([]);
		expect(splitMergeDiffPatch(undefined, false)).toEqual([]);
		expect(splitMergeDiffPatch('   \n  ', false)).toEqual([]);
	});

	it('marks only the last file truncated, and says so in its rows', () => {
		const files = splitMergeDiffPatch(patch, true);

		expect(files[0].truncated).toBe(false);
		expect(files[1].truncated).toBe(true);
		// The cap is a byte slice, so the last file can stop mid-hunk; saying so beats
		// rendering a file that looks complete and is not.
		expect(files[1].rows.at(-1)?.kind).toBe('truncated');
		// And the rows before it survive, which the first version of this parser lost.
		expect(files[1].rows.some((row) => row.kind === 'add')).toBe(true);
	});
});

describe('parseDiffRows', () => {
	it('numbers lines per side, and only on the side they exist on', () => {
		const files = splitMergeDiffPatch(patch, false);
		const rows = files[0].rows;

		// The hunk starts at 10; three context lines precede the change, so the removed
		// line is old 13 and the first added line is new 13.
		const added = rows.find((row) => row.kind === 'add');
		expect(added).toMatchObject({ oldLine: null, newLine: 13 });

		const removed = rows.find((row) => row.kind === 'remove');
		expect(removed).toMatchObject({ oldLine: 13, newLine: null });

		// A context line is on both sides and advances both.
		const context = rows.filter((row) => row.kind === 'context');
		expect(context[0]).toMatchObject({ oldLine: 10, newLine: 10 });
		expect(context[1]).toMatchObject({ oldLine: 11, newLine: 11 });
	});

	it('reads a hunk header as counts, defaulting a bare range to one line', () => {
		const rows = parseDiffRows('@@ -1,4 +1,6 @@\n a\n@@ -9 +12 @@\n b\n', false);

		const hunks = rows.filter((row) => row.kind === 'hunk');
		expect(hunks[0]).toMatchObject({ oldStart: 1, oldLines: 4, newStart: 1, newLines: 6 });
		expect(hunks[1]).toMatchObject({ oldStart: 9, oldLines: 1, newStart: 12, newLines: 1 });
	});

	it('treats a stripped leading space as context rather than a header', () => {
		// A patch that has been through an editor loses the space a context line is
		// marked with, and an empty line is context, not a mode change.
		const rows = parseDiffRows('@@ -1,2 +1,2 @@\n\tif x {\n\n\t}', false);

		// A stripped context line and a bare empty line are both context; the one
		// starting with `+` is not, and a context line is not a header either.
		expect(rows.filter((row) => row.kind === 'context').map((row) => row.text)).toEqual([
			'\tif x {',
			'',
			'\t}'
		]);
		expect(rows.filter((row) => row.kind === 'add')).toHaveLength(0);
	});

	it('keeps mode changes and no-newline markers out of the body rows', () => {
		const rows = parseDiffRows(
			[
				'diff --git a/x b/x',
				'old mode 100644',
				'new mode 100755',
				'--- a/x',
				'+++ b/x',
				'@@ -1 +1 @@',
				'-a',
				'\\ No newline at end of file',
				'+b',
				''
			].join('\n'),
			false
		);

		expect(rows.filter((row) => row.kind === 'meta').map((row) => row.text)).toEqual([
			'diff --git a/x b/x',
			'old mode 100644',
			'new mode 100755',
			'--- a/x',
			'+++ b/x',
			'\\ No newline at end of file'
		]);
	});
});

describe('rowAnchor', () => {
	it('anchors an added line to the new side and a removed one to the old', () => {
		const files = splitMergeDiffPatch(patch, false);
		const rows = files[0].rows;

		const add = rows.find((row) => row.kind === 'add');
		const remove = rows.find((row) => row.kind === 'remove');
		const context = rows.find((row) => row.kind === 'context');

		expect(rowAnchor(files[0], add!)).toMatchObject({ side: 'new', line: 13 });
		expect(rowAnchor(files[0], remove!)).toMatchObject({ side: 'old', line: 13 });
		// A context line is on both sides; the new side is the one a reviewer reads
		// and the one that survives the change.
		expect(rowAnchor(files[0], context!)).toMatchObject({ side: 'new', line: 10 });
	});

	it('refuses to anchor a row that is not a line', () => {
		const files = splitMergeDiffPatch(patch, false);
		const hunk = files[0].rows.find((row) => row.kind === 'hunk');
		const meta = files[0].rows.find((row) => row.kind === 'meta');

		expect(rowAnchor(files[0], hunk!)).toBeNull();
		expect(rowAnchor(files[0], meta!)).toBeNull();
	});
});

describe('buildMergeDiffFiles and the path filter', () => {
	it('labels each file from the stat, and leaves a renamed file unlabelled', () => {
		const files = buildMergeDiffFiles({ diff: patch, stat });

		expect(files[0].statLabel).toBe('+3 -2');
		// `+++` with no minus is three additions and zero deletions, and saying so is
		// what the legacy bar-rendering did too.
		expect(files[1].statLabel).toBe('+3 -0');
		// Git prints a rename as `{old => new}` in the stat, which matches no path,
		// so the label is empty rather than wrong.
		const renamed = buildMergeDiffFiles({
			diff: 'diff --git a/old.go b/new.go\n--- a/old.go\n+++ b/new.go\n@@ -1 +1 @@\n-a\n+b\n',
			stat: ' {old.go => new.go} | 2 +-\n'
		});
		expect(renamed[0].statLabel).toBe('');
	});

	it('filters by substring, case-insensitively', () => {
		const files = buildMergeDiffFiles({ diff: patch, stat });

		expect(filterDiffFiles(files, 'console').map((file) => file.path)).toEqual([
			'internal/console/handlers.go'
		]);
		expect(filterDiffFiles(files, '.TS')).toHaveLength(1);
		expect(filterDiffFiles(files, '  ')).toHaveLength(2);
		expect(filterDiffFiles(files, 'nothing')).toHaveLength(0);
	});
});
