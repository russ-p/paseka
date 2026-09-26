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
