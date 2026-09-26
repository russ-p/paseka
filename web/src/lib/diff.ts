/**
 * A unified diff as data.
 *
 * The legacy console handed its raw patch to diff2html and then *scraped the
 * generated DOM* for line numbers to anchor a review comment on — `td.d2h-ins
 * .line-num2` and friends. That couples the comment anchors to a third party's
 * private markup, and it is why a diff2html upgrade would have been a review-page
 * change. Here the patch is parsed once into rows that carry their own line
 * numbers, so anchoring a comment is reading a field rather than querying a
 * selector, and the body renders as Svelte text nodes with no `@html`.
 */
export interface DiffStatEntry {
	/** The `12` in `path | 12 +++---`. */
	changes: number;
	adds: number;
	deletes: number;
}

export type DiffRow =
	| { kind: 'meta'; text: string }
	| {
			kind: 'hunk';
			text: string;
			oldStart: number;
			oldLines: number;
			newStart: number;
			newLines: number;
	  }
	| {
			kind: 'add' | 'remove' | 'context';
			text: string;
			/** `null` on a side the line does not exist on. */
			oldLine: number | null;
			newLine: number | null;
	  }
	| { kind: 'binary'; text: string }
	| { kind: 'truncated'; text: string };

/** One file's share of a patch, already split into renderable rows. */
export interface DiffFile {
	/** The `b/` side, which is the path a reviewer recognises after a rename. */
	path: string;
	/** The `a/` side when it differs, so a rename shows both. */
	oldPath?: string;
	binary: boolean;
	/** The whole patch was cut at the server's byte cap and this is the last file. */
	truncated: boolean;
	rows: DiffRow[];
	statLabel: string;
}

/** The side of a diff a line exists on, which is what a comment is anchored to. */
export type DiffSide = 'old' | 'new';

/**
 * One side of a row in a side-by-side layout. `tone` is empty on a *filler* cell —
 * the blank a remove with no matching add leaves on the right — which is how the
 * renderer knows to draw nothing rather than an empty numbered line.
 */
export interface DiffCell {
	text: string;
	line: number | null;
	tone: '' | 'add' | 'remove' | 'context';
	/** The side this cell is on, or `null` on a filler. */
	side: DiffSide | null;
}

export type SplitRow =
	| { kind: 'banner'; banner: 'meta' | 'hunk' | 'binary' | 'truncated'; text: string }
	| { kind: 'split'; left: DiffCell; right: DiffCell };

export interface DiffAnchor {
	path: string;
	side: DiffSide;
	line: number;
	snippet: string;
}

/** `+12 -3`, or `7 lines` when the stat bar is empty, or nothing. */
export function diffStatLabel(entry: DiffStatEntry | undefined): string {
	if (!entry) return '';
	if (entry.adds > 0 || entry.deletes > 0) return `+${entry.adds} -${entry.deletes}`;
	return entry.changes > 0 ? `${entry.changes} lines` : '';
}

/**
 * `git diff --stat` into per-path counts. The bar after the `|` is the only place
 * the added/removed split exists — the patch itself does not carry totals — so it
 * has to be read from the stat, not from the hunks.
 */
export function parseMergeDiffStat(stat: string | undefined): Record<string, DiffStatEntry> {
	const counts: Record<string, DiffStatEntry> = {};
	if (!stat) return counts;
	for (const line of stat.split('\n')) {
		// `path | 12 +++---`, with the bar's length scaled to fit the terminal.
		const match = line.match(/^\s*(.+?)\s+\|\s+(\d+)\s*([+-]*)/);
		if (!match) continue;
		const bar = match[3] ?? '';
		counts[match[1].trim()] = {
			changes: Number.parseInt(match[2], 10),
			adds: (bar.match(/\+/g) ?? []).length,
			deletes: (bar.match(/-/g) ?? []).length
		};
	}
	return counts;
}

/**
 * The path out of a `diff --git a/x b/x` header. The groups are non-greedy and
 * anchored on the ` b/`, which is what git itself does; a path containing the
 * literal ` b/` would mis-split, and that is the same trade git's parsers make.
 */
function headerPaths(header: string): { oldPath?: string; path: string } | null {
	const match = header.match(/^diff --git a\/(.+?) b\/(.+?)(?:\n|$)/);
	if (!match) return null;
	return match[1] === match[2] ? { path: match[2] } : { oldPath: match[1], path: match[2] };
}

const metaPrefixes = [
	'index ',
	'old mode',
	'new mode',
	'new file mode',
	'deleted file mode',
	'similarity index',
	'dissimilarity index',
	'copy from',
	'copy to',
	'rename from',
	'rename to'
];

/** One file's patch into rows with real line numbers. */
export function parseDiffRows(patch: string, truncated: boolean): DiffRow[] {
	const rows: DiffRow[] = [];
	let oldLine: number | null = null;
	let newLine: number | null = null;
	/**
	 * What the hunk header says is left on each side. A patch ends in a newline, so
	 * splitting it yields a trailing empty line that is not a line of the file at all
	 * — without this, every diff renders a phantom context row past the end of its
	 * last hunk, numbered one past the end of the file. The header is git's own count,
	 * so it is what decides when a hunk is over.
	 */
	let oldLeft = 0;
	let newLeft = 0;

	for (const line of patch.split('\n')) {
		// A hunk header resets both counters; without one, a line's number is
		// whatever the previous hunk left behind, which is how a line gets anchored
		// to the wrong place.
		const hunk = line.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/);
		if (hunk) {
			oldLine = Number.parseInt(hunk[1], 10);
			newLine = Number.parseInt(hunk[3], 10);
			oldLeft = hunk[2] === undefined ? 1 : Number.parseInt(hunk[2], 10);
			newLeft = hunk[4] === undefined ? 1 : Number.parseInt(hunk[4], 10);
			rows.push({
				kind: 'hunk',
				text: line,
				oldStart: oldLine,
				oldLines: oldLeft,
				newStart: newLine,
				newLines: newLeft
			});
			continue;
		}
		if (line.startsWith('Binary files ') || line.startsWith('GIT binary patch')) {
			rows.push({ kind: 'binary', text: line });
			continue;
		}
		if (line.startsWith('\\')) {
			// "\ No newline at end of file" belongs to the line above it.
			rows.push({ kind: 'meta', text: line });
			continue;
		}
		if (
			line.startsWith('diff --git ') ||
			line.startsWith('--- ') ||
			line.startsWith('+++ ') ||
			metaPrefixes.some((prefix) => line.startsWith(prefix))
		) {
			rows.push({ kind: 'meta', text: line });
			continue;
		}
		if (line.startsWith('+')) {
			// Past the hunk's own count this is not a line of the file. Skipping it
			// rather than rendering it is what stops a patch's trailing newline from
			// becoming a numbered row.
			if (newLeft <= 0) continue;
			newLeft -= 1;
			rows.push({ kind: 'add', text: line.slice(1), oldLine: null, newLine });
			newLine = newLine === null ? null : newLine + 1;
			continue;
		}
		if (line.startsWith('-')) {
			if (oldLeft <= 0) continue;
			oldLeft -= 1;
			rows.push({ kind: 'remove', text: line.slice(1), oldLine, newLine: null });
			oldLine = oldLine === null ? null : oldLine + 1;
			continue;
		}
		// A context line is a leading space, but a patch that has been through an
		// editor can have that space stripped, and an empty line is still context
		// rather than a header.
		//
		// A context line is present in *both* versions, so it is only a line if both
		// hunk counts still have one. Guarding per side would let a remaining old-side
		// budget carry a row whose new side is already past its count, and that row
		// would be numbered past the end of the file.
		if (oldLeft <= 0 || newLeft <= 0) continue;
		rows.push({
			kind: 'context',
			text: line.startsWith(' ') ? line.slice(1) : line,
			oldLine,
			newLine
		});
		if (oldLeft > 0) oldLeft -= 1;
		if (newLeft > 0) newLeft -= 1;
		if (oldLine !== null) oldLine += 1;
		if (newLine !== null) newLine += 1;
	}

	if (truncated) {
		// The cap is a byte-slice cut, so the last file can stop mid-hunk. Saying so
		// beats rendering a file that looks complete and is not.
		rows.push({
			kind: 'truncated',
			text: 'Diff truncated — open the worktree locally for the rest of this file.'
		});
	}
	return rows;
}

/**
 * A patch into files. Split on the literal `diff --git ` header, the one line every
 * git patch guarantees; `+++ b/` is not a safe delimiter because a context line
 * can begin with the same three characters.
 */
export function splitMergeDiffPatch(
	diff: string | undefined,
	truncated: boolean
): DiffFile[] {
	if (!diff || diff.trim() === '') return [];
	// The patch is kept until the truncation flag is known, because the flag
	// belongs to the last file and its rows have to be re-parsed with the
	// truncation notice attached.
	const parsed: { paths: ReturnType<typeof headerPaths>; patch: string; binary: boolean }[] = [];
	for (const part of diff.split(/^diff --git /m)) {
		if (part.trim() === '') continue;
		// `split` consumed the delimiter, so put it back: the patch has to stay a
		// valid diff for the header parse and for anything that reads it later.
		const patch = part.startsWith('diff --git ') ? part : `diff --git ${part}`;
		parsed.push({
			paths: headerPaths(patch.split('\n')[0] ?? ''),
			patch,
			binary: /(^|\n)(Binary files |GIT binary patch)/.test(patch)
		});
	}
	return parsed.map((entry, index) => {
		// Only the last file can be the cut one, and only the server knows that it
		// cut at all — the cap is a byte slice, so a file can stop mid-hunk.
		const cut = truncated && index === parsed.length - 1;
		return {
			path: entry.paths?.path ?? `file-${index}`,
			oldPath: entry.paths?.oldPath,
			binary: entry.binary,
			truncated: cut,
			rows: parseDiffRows(entry.patch, cut),
			// Filled in by `buildMergeDiffFiles`, which is the only caller that has
			// the stat to read the per-file counts from.
			statLabel: ''
		};
	});
}

/** A patch plus its stat into the file list the viewer navigates. */
export function buildMergeDiffFiles(view: {
	diff?: string;
	stat?: string;
	truncated?: boolean;
}): DiffFile[] {
	const counts = parseMergeDiffStat(view.stat);
	const files = splitMergeDiffPatch(view.diff, view.truncated ?? false);
	return files.map((file) => ({ ...file, statLabel: diffStatLabel(counts[file.path]) }));
}

/** Case-insensitive substring, the way a path filter is read. */
export function filterDiffFiles(files: DiffFile[], filter: string): DiffFile[] {
	const needle = filter.trim().toLowerCase();
	if (needle === '') return files;
	return files.filter((file) => file.path.toLowerCase().includes(needle));
}

/**
 * The comment anchor for a row, or `null` for a row that does not exist in a file:
 * a hunk header and a mode change are not lines anyone can leave a note on.
 */
export function rowAnchor(file: DiffFile, row: DiffRow): DiffAnchor | null {
	if (row.kind === 'add' && row.newLine !== null) {
		return { path: file.path, side: 'new', line: row.newLine, snippet: row.text };
	}
	if (row.kind === 'remove' && row.oldLine !== null) {
		return { path: file.path, side: 'old', line: row.oldLine, snippet: row.text };
	}
	if (row.kind === 'context' && row.newLine !== null) {
		// A context line is on both sides; the new side is the one a reviewer is
		// reading, and it is the side that survives the change.
		return { path: file.path, side: 'new', line: row.newLine, snippet: row.text };
	}
	return null;
}

function cell(text: string, line: number | null, tone: DiffCell['tone'], side: DiffSide): DiffCell {
	return { text, line, tone, side };
}

/** The blank half of a row where the other side has no line. */
function filler(): DiffCell {
	return { text: '', line: null, tone: '', side: null };
}

/**
 * Rows into side-by-side pairs.
 *
 * A unified patch is already a sequence of removals followed by the additions that
 * replace them, so the split layout pairs each run one-for-one and leaves a filler
 * where one side runs out. That is the whole algorithm, and it is why a side-by-side
 * view needs no per-file state: the pairing is a property of the patch, not of the
 * renderer.
 *
 * A context line occupies both halves. In the unified view it is anchored to the new
 * side only, because there is one row; here each half is its own cell, so a note can
 * be left against the old line it is about — which is the whole reason to read a
 * diff side-by-side rather than unified.
 */
export function pairDiffRows(rows: DiffRow[]): SplitRow[] {
	const out: SplitRow[] = [];
	let index = 0;
	while (index < rows.length) {
		const row = rows[index];
		if (row.kind === 'meta' || row.kind === 'hunk' || row.kind === 'binary' || row.kind === 'truncated') {
			out.push({ kind: 'banner', banner: row.kind, text: row.text });
			index += 1;
			continue;
		}
		if (row.kind === 'context') {
			out.push({
				kind: 'split',
				left: cell(row.text, row.oldLine, 'context', 'old'),
				right: cell(row.text, row.newLine, 'context', 'new')
			});
			index += 1;
			continue;
		}
		const removes: DiffRow[] = [];
		while (index < rows.length && rows[index].kind === 'remove') {
			removes.push(rows[index]);
			index += 1;
		}
		const adds: DiffRow[] = [];
		while (index < rows.length && rows[index].kind === 'add') {
			adds.push(rows[index]);
			index += 1;
		}
		// Whatever is longer sets the row count, so a run of five removals replaced by
		// two additions is five rows rather than losing three lines.
		for (let n = 0; n < Math.max(removes.length, adds.length); n += 1) {
			const removed = removes[n];
			const added = adds[n];
			out.push({
				kind: 'split',
				left:
					removed && removed.kind === 'remove'
						? cell(removed.text, removed.oldLine, 'remove', 'old')
						: filler(),
				right:
					added && added.kind === 'add' ? cell(added.text, added.newLine, 'add', 'new') : filler()
			});
		}
	}
	return out;
}

/**
 * The anchor for one half of a split row, or `null` for a filler. A filler has no line
 * on its side, so there is nothing to point a note at.
 */
export function cellAnchor(path: string, side: DiffCell): DiffAnchor | null {
	if (side.line === null || side.side === null) return null;
	return { path, side: side.side, line: side.line, snippet: side.text };
}
