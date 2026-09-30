import { describe, expect, it } from 'vitest';
import { jsonTokenClasses, tokenizeJson, type TokenRole } from './tokenize';

/** The theme slot a class speaks for, so the distinctness test reads the constraint, not the palette. */
const slotOf = (role: TokenRole): string => jsonTokenClasses[role]!.replace(/^text-/, '');

const rolesOf = (source: string): TokenRole[] =>
	tokenizeJson(source)
		.filter((token) => token.role !== 'text')
		.map((token) => token.role);

/** What the feed actually renders: `JSON.stringify` of a `ProtocolEvent`. */
const envelope = JSON.stringify(
	{
		protocolVersion: '1',
		traceId: 'trace-01',
		agentId: 'planner-01',
		seq: 12,
		type: 'task.ready',
		createdAt: '2026-09-20T09:15:06Z',
		payload: { branch: 'paseka/trace-01', retries: 2, dryRun: false, note: null, files: ['a.go'] }
	},
	null,
	2
);

describe('tokenizeJson', () => {
	it('names an object member by the colon after it, not by where it sits', () => {
		expect(rolesOf('{"traceId":"trace-01"}')).toEqual(['punct', 'key', 'punct', 'string', 'punct']);
		expect(rolesOf('["traceId"]')).toEqual(['punct', 'string', 'punct']);
	});

	it('reads a key across the newline JSON.stringify puts between it and its colon', () => {
		expect(rolesOf('{\n  "seq"\n  : 12\n}')).toEqual(['punct', 'key', 'punct', 'number', 'punct']);
	});

	it('splits a real envelope into the six roles and no others', () => {
		expect(new Set(rolesOf(envelope))).toEqual(
			new Set<TokenRole>(['key', 'string', 'number', 'literal', 'punct'])
		);
	});

	it('takes a number whole, sign, fraction, and exponent included', () => {
		for (const source of ['0', '-7', '3.25', '-1.5e-3', '1e+10', '9007199254740993']) {
			expect(tokenizeJson(source)).toEqual([{ role: 'number', text: source }]);
		}
	});

	it('reads true, false, and null as literals but not a word that merely resembles one', () => {
		expect(rolesOf('[true,false,null]')).toEqual(['punct', 'literal', 'punct', 'literal', 'punct', 'literal', 'punct']);
		// `JSON.stringify` drops `undefined`, but a hand-built payload can carry the word,
		// and colouring it as `null` would say something the envelope did not say.
		expect(rolesOf('[undefined,nullify]')).toEqual(['punct', 'punct', 'punct']);
	});

	it('keeps an escaped quote inside its string rather than ending it there', () => {
		expect(tokenizeJson('{"summary":"said \\"go\\" twice"}')).toEqual([
			{ role: 'punct', text: '{' },
			{ role: 'key', text: '"summary"' },
			{ role: 'punct', text: ':' },
			{ role: 'string', text: '"said \\"go\\" twice"' },
			{ role: 'punct', text: '}' }
		]);
	});

	it('groups the punctuation a pretty-printed envelope puts on one line', () => {
		expect(tokenizeJson('{\n  "a": 1\n},\n')).toEqual([
			{ role: 'punct', text: '{' },
			{ role: 'text', text: '\n  ' },
			{ role: 'key', text: '"a"' },
			{ role: 'punct', text: ':' },
			{ role: 'text', text: ' ' },
			{ role: 'number', text: '1' },
			{ role: 'text', text: '\n' },
			{ role: 'punct', text: '},' },
			{ role: 'text', text: '\n' }
		]);
	});

	it('reads a line and a block comment, which JSON has but JSON.stringify never writes', () => {
		expect(tokenizeJson('// note\n1')).toEqual([
			{ role: 'comment', text: '// note' },
			{ role: 'text', text: '\n' },
			{ role: 'number', text: '1' }
		]);
		expect(tokenizeJson('/* a */1')).toEqual([
			{ role: 'comment', text: '/* a */' },
			{ role: 'number', text: '1' }
		]);
	});
});

describe('the lossless invariant', () => {
	// The property the component depends on: it renders `tokens` and nothing else, so if
	// the concatenation is not the source the operator is reading something other than
	// what was published. Every case here is one that could plausibly reach a raw view.
	it.each([
		['a real envelope', envelope],
		['an empty string', ''],
		['whitespace only', '  \n\t'],
		['an unterminated string', '{"summary": "no closing quote'],
		['an unterminated block comment', '/* forever'],
		['a stray backslash', '"trailing\\'],
		['a minus before nothing', '-'],
		['input that is not JSON at all', 'not json, just prose with "quotes" and 42 in it'],
		['markup from an author-supplied payload', '{"summary":"<script>alert(1)</script>"}'],
		['a lone surrogate pair', '{"bee":"\ud83e\udd89"}']
	])('reassembles %s exactly', (_case, source) => {
		expect(tokenizeJson(source).map((token) => token.text).join('')).toBe(source);
	});

	it('leaves no token empty, because an empty span is a node that renders nothing', () => {
		expect(tokenizeJson(envelope).every((token) => token.text.length > 0)).toBe(true);
	});
});

describe('jsonTokenClasses', () => {
	it('gives every role a slot, so a new role cannot ship uncolored by accident', () => {
		const roles: TokenRole[] = ['key', 'string', 'number', 'literal', 'comment', 'punct', 'text'];
		expect(Object.keys(jsonTokenClasses).sort()).toEqual([...roles].sort());
		roles.filter((role) => role !== 'text').forEach((role) => {
			expect(jsonTokenClasses[role]).toBeTruthy();
		});
	});

	it('puts six roles on six different theme slots', () => {
		const slots = (['key', 'string', 'number', 'literal', 'comment', 'punct'] as TokenRole[]).map(slotOf);
		expect(new Set(slots).size).toBe(slots.length);
		expect(slots).toEqual(['primary', 'success', 'warning', 'secondary', 'base-content/50', 'base-content/70']);
	});

	it('stays off info, because info is the same value as primary in all four Catppuccin themes', () => {
		// `app.css` writes one hex for both in every Catppuccin block, so `text-info` here
		// would render identically to `text-primary` on four of the six themes the console
		// ships — a map that looks like it has eight colours while showing six. The premise
		// is a fact about the palette, not about this lexer, so it lives in the comment on
		// the map; this test guards only what the lexer must keep doing.
		expect(Object.values(jsonTokenClasses)).not.toContain('text-info');
		expect(Object.values(jsonTokenClasses).filter(Boolean)).toHaveLength(6);
	});
});
