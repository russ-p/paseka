import { describe, expect, it } from 'vitest';
import { statusTone, statusToneClasses } from './status';

describe('statusTone', () => {
	it.each([
		['running', 'info'],
		['connected', 'info'],
		['live', 'info'],
		['active', 'info'],
		['approved', 'success'],
		['waiting_review', 'warning'],
		['reconnecting', 'warning'],
		['failed', 'error'],
		['disconnected', 'error'],
		['idle', 'neutral'],
		['stopped', 'neutral'],
		['unexpected', 'neutral']
	])('maps %s to %s', (status, expected) => {
		expect(statusTone(status)).toBe(expected);
	});

	it('exposes a static DaisyUI badge class for every tone', () => {
		expect(Object.values(statusToneClasses)).toEqual(
			expect.arrayContaining(['badge-info', 'badge-success', 'badge-warning', 'badge-error', 'badge-neutral'])
		);
	});
});

describe('domain statuses the trace surfaces introduced', () => {
	it.each([
		['queued', 'info'],
		['completed', 'success'],
		['announced', 'success'],
		['low', 'warning'],
		['blocked', 'error'],
		['cancelled', 'error'],
		['planned', 'neutral'],
		['standing', 'neutral'],
		['staged', 'neutral']
	])('maps %s to %s', (status, expected) => {
		expect(statusTone(status)).toBe(expected);
	});

	it('ignores case and padding so a payload cannot slip past the table', () => {
		expect(statusTone('  Waiting_Review ')).toBe('warning');
		expect(statusTone('BLOCKED')).toBe('error');
	});
});

describe('domain statuses the git route introduced', () => {
	// `current` is a branch *label*; the tone travels as `active`, so it has no row of its own.
	it.each([
		['clean', 'success'],
		['merged', 'success'],
		['dirty', 'warning'],
		['leftover', 'warning']
	])('maps %s to %s', (status, expected) => {
		expect(statusTone(status)).toBe(expected);
	});
});
