/**
 * A JSON lexer, not a parser. It exists so a raw event envelope can be read as
 * structure without ever risking the text: whatever it does not recognise stays
 * plain, and the concatenation of its tokens is the source byte for byte. A
 * highlighter that could reject or reorder a payload would be a highlighter that
 * hides evidence, so `tokenizeJson` never validates and never throws — a payload
 * that is not JSON renders as itself.
 */

export type TokenRole = 'key' | 'string' | 'number' | 'literal' | 'comment' | 'punct' | 'text';

export interface Token {
	role: TokenRole;
	text: string;
}

const literals = new Set(['true', 'false', 'null']);
const punctuation = new Set(['{', '}', '[', ']', ',', ':']);

/** The JSON number grammar exactly, so `1e-10` is one token rather than three. */
const numberAt = /-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?/y;

const isWhitespace = (char: string): boolean => /\s/.test(char);
const isDigit = (char: string): boolean => char >= '0' && char <= '9';
const isIdentStart = (char: string): boolean => /[A-Za-z_]/.test(char);
const isIdentPart = (char: string): boolean => /[A-Za-z0-9_]/.test(char);

/**
 * A string is a key by what follows it, not by where it sits: JSON has no separate
 * token for the name of a field, so `"a"` in `{"a": 1}` and `"a"` in `["a"]` are the
 * same characters and only the colon tells them apart. Whitespace between the two is
 * allowed because that is how `JSON.stringify(value, null, 2)` writes it.
 */
function isKey(source: string, from: number): boolean {
	let index = from;
	while (index < source.length && isWhitespace(source[index])) index += 1;
	return source[index] === ':';
}

/** A backslash escapes whatever follows it, including the quote that would end the string. */
function readString(source: string, start: number): [string, number] {
	let index = start + 1;
	while (index < source.length) {
		const char = source[index];
		if (char === '\\') {
			index += 2;
			continue;
		}
		if (char === '"') return [source.slice(start, index + 1), index + 1];
		index += 1;
	}
	// Unterminated: the rest of the source is the token, so nothing is lost.
	return [source.slice(start), source.length];
}

function readComment(source: string, start: number): [string, number] {
	if (source[start + 1] === '/') {
		const end = source.indexOf('\n', start);
		return end === -1 ? [source.slice(start), source.length] : [source.slice(start, end), end];
	}
	const end = source.indexOf('*/', start + 2);
	return end === -1 ? [source.slice(start), source.length] : [source.slice(start, end + 2), end + 2];
}

function readIdent(source: string, start: number): [string, number] {
	let index = start;
	while (index < source.length && isIdentPart(source[index])) index += 1;
	return [source.slice(start, index), index];
}

export function tokenizeJson(source: string): Token[] {
	const tokens: Token[] = [];
	let index = 0;

	const push = (role: TokenRole, text: string) => {
		if (text !== '') tokens.push({ role, text });
	};

	while (index < source.length) {
		const char = source[index];

		if (isWhitespace(char)) {
			const start = index;
			while (index < source.length && isWhitespace(source[index])) index += 1;
			push('text', source.slice(start, index));
			continue;
		}

		if (char === '"') {
			const [text, next] = readString(source, index);
			index = next;
			push(isKey(source, next) ? 'key' : 'string', text);
			continue;
		}

		if (isDigit(char) || (char === '-' && isDigit(source[index + 1] ?? ''))) {
			numberAt.lastIndex = index;
			const match = numberAt.exec(source);
			if (match) {
				push('number', match[0]);
				index = numberAt.lastIndex;
				continue;
			}
		}

		if (isIdentStart(char)) {
			const [text, next] = readIdent(source, index);
			index = next;
			// `undefined` and a truncated word are not JSON literals, and colouring
			// them as `null` would say something the payload did not say.
			push(literals.has(text) ? 'literal' : 'text', text);
			continue;
		}

		if (punctuation.has(char)) {
			// Grouped, because pretty-printed JSON puts `},` and `],` on one line and
			// one span per character would double the element count of every envelope.
			const start = index;
			while (index < source.length && punctuation.has(source[index])) index += 1;
			push('punct', source.slice(start, index));
			continue;
		}

		if (char === '/' && (source[index + 1] === '/' || source[index + 1] === '*')) {
			const [text, next] = readComment(source, index);
			index = next;
			push('comment', text);
			continue;
		}

		push('text', char);
		index += 1;
	}

	return tokens;
}

/**
 * Six roles on six different theme slots, rather than a palette per theme. The
 * restriction is real and worth stating: `primary` and `info` are the same value in
 * all four Catppuccin themes (`app.css` writes one hex for both), so anything mapped
 * to `info` would render identically to `primary` on four of the six themes the
 * console ships, and the map would look like it had eight colours while showing six.
 */
export const jsonTokenClasses = {
	key: 'text-primary',
	string: 'text-success',
	number: 'text-warning',
	literal: 'text-secondary',
	comment: 'text-base-content/50',
	punct: 'text-base-content/70',
	/** Unclassified text carries no class at all, so it inherits the block's own colour. */
	text: undefined
} as const satisfies Record<TokenRole, string | undefined>;
