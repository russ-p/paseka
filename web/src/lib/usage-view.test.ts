import { afterEach, describe, expect, it } from 'vitest';
import {
	clearUsageFilters,
	hasUsageFilters,
	readUsageView,
	toggleUsageFilter,
	usageDayRange,
	usageFilterCount,
	usageFilterValues,
	usagePresetRange,
	withUsageFilter,
	withoutUsageFilter,
	writeUsageView,
	type UsageFilters
} from './usage-view';
import { usageAxisEntry, usageReport } from '../tests/fixtures';

const now = new Date(2026, 8, 30, 12, 0, 0);

function at(search: string): void {
	window.history.replaceState(null, '', `/usage${search}`);
}

function filters(overrides: Partial<UsageFilters> = {}): UsageFilters {
	return { adapter: [], model: [], bee: [], purpose: [], ...overrides };
}

describe('usage view URL', () => {
	afterEach(() => {
		window.history.replaceState(null, '', '/');
	});

	it('reads an empty address as the 30-day view with no filters', () => {
		at('');

		const view = readUsageView(now);

		expect(view).toEqual({
			preset: 30,
			...usagePresetRange(30, now),
			day: null,
			axis: 'components',
			filters: filters()
		});
	});

	it('round-trips the range, axis, and every repeated filter', () => {
		at('');

		const written = {
			since: '2026-09-01',
			until: '2026-09-15',
			day: null,
			axis: 'model' as const,
			filters: filters({
				adapter: ['opencode', 'cursor'],
				model: ['anthropic/claude-sonnet'],
				bee: ['builder'],
				purpose: ['work', 'closing']
			})
		};
		writeUsageView(written);

		expect(window.location.search).toBe(
			'?since=2026-09-01&until=2026-09-15&axis=model' +
				'&adapter=opencode&adapter=cursor' +
				'&model=anthropic%2Fclaude-sonnet' +
				'&bee=builder' +
				'&purpose=work&purpose=closing'
		);
		expect(readUsageView(now)).toEqual({ preset: 'custom', ...written });
	});

	it('round-trips a drilled day beside the range it was narrowed from', () => {
		at('');
		writeUsageView({
			since: '2026-09-01',
			until: '2026-10-01',
			day: '2026-09-25',
			axis: 'components',
			filters: filters()
		});

		expect(new URLSearchParams(window.location.search).get('day')).toBe('2026-09-25');
		const view = readUsageView(now);
		// The range stays the context; the day is a second, reversible narrowing.
		expect(view).toMatchObject({ since: '2026-09-01', until: '2026-10-01', day: '2026-09-25' });

		writeUsageView({ since: '2026-09-01', until: '2026-10-01', day: null, axis: 'components', filters: filters() });
		expect(new URLSearchParams(window.location.search).has('day')).toBe(false);
		expect(readUsageView(now).day).toBeNull();
	});

	it('drops a day the calendar cannot hold rather than narrowing to it', () => {
		at('?since=2026-09-01&until=2026-10-01&day=2026-02-30');

		expect(readUsageView(now).day).toBeNull();
	});

	it('narrows a drill-down to one whole local day', () => {
		expect(usageDayRange('2026-09-25')).toEqual({
			since: '2026-09-25',
			until: '2026-09-26'
		});
		expect(usageDayRange('2026-02-30')).toBeNull();
		expect(usageDayRange('yesterday')).toBeNull();
	});

	it('names a preset range it recognises and treats any other as custom', () => {
		const seven = usagePresetRange(7, now);
		at(`?since=${seven.since}&until=${seven.until}`);

		expect(readUsageView(now).preset).toBe(7);

		at('?since=2026-08-01&until=2026-08-10');
		expect(readUsageView(now).preset).toBe('custom');
	});

	it('falls back to the default range rather than trusting an impossible one', () => {
		for (const search of [
			'?since=2026-09-10&until=2026-09-01',
			'?since=yesterday&until=2026-09-01',
			'?since=2026-02-30&until=2026-03-01',
			'?since=2026-09-01'
		]) {
			at(search);
			const view = readUsageView(now);
			expect(view.preset).toBe(30);
			expect(view.since).toBe(usagePresetRange(30, now).since);
		}
	});

	it('drops a filter value that is blank or repeated, and keeps the rest verbatim', () => {
		at('?adapter=opencode&adapter=&adapter=opencode&adapter=%20cursor%20');

		expect(readUsageView(now).filters.adapter).toEqual(['opencode', 'cursor']);
	});

	it('ignores an unknown axis rather than putting it on the chart', () => {
		at('?axis=intent');

		expect(readUsageView(now).axis).toBe('components');
	});

	it('leaves the path, the hash, and params it does not own alone', () => {
		window.history.replaceState(null, '', '/usage#breakdown?trace=trace-01');
		window.history.replaceState(null, '', '/usage?trace=trace-01#breakdown');

		writeUsageView({
			since: '2026-09-01',
			until: '2026-09-15',
			day: null,
			axis: 'bee',
			filters: filters({ bee: ['builder'] })
		});

		expect(window.location.pathname).toBe('/usage');
		expect(window.location.hash).toBe('#breakdown');
		expect(new URLSearchParams(window.location.search).get('trace')).toBe('trace-01');
	});

	it('costs no history write when the view did not change', () => {
		at('?since=2026-09-01&until=2026-09-15');
		const before = window.history.length;

		writeUsageView({
			since: '2026-09-01',
			until: '2026-09-15',
			day: null,
			axis: 'components',
			filters: filters()
		});

		expect(window.history.length).toBe(before);
	});
});

describe('usage filter set', () => {
	it('adds and removes values without mutating the previous set', () => {
		const empty = filters();

		const one = withUsageFilter(empty, 'model', '  openai/gpt-5  ');
		expect(one.model).toEqual(['openai/gpt-5']);
		expect(empty.model).toEqual([]);

		// The same value twice is one value, and costs no new object.
		expect(withUsageFilter(one, 'model', 'openai/gpt-5')).toBe(one);
		expect(withUsageFilter(one, 'model', '   ')).toBe(one);

		const two = withUsageFilter(one, 'model', 'anthropic/claude-sonnet');
		expect(two.model).toEqual(['openai/gpt-5', 'anthropic/claude-sonnet']);

		expect(withoutUsageFilter(two, 'model', 'openai/gpt-5').model).toEqual([
			'anthropic/claude-sonnet'
		]);
		expect(withoutUsageFilter(two, 'bee', 'builder')).toBe(two);

		// A breakdown row's click is the same control in two directions: one
		// click adds the key, the next removes it, and a blank key is no click.
		expect(toggleUsageFilter(one, 'model', 'openai/gpt-5')).toEqual(filters());
		expect(toggleUsageFilter(one, 'model', '  openai/gpt-5  ')).toEqual(filters());
		expect(toggleUsageFilter(one, 'model', 'anthropic/claude-sonnet').model).toEqual([
			'openai/gpt-5',
			'anthropic/claude-sonnet'
		]);
		expect(toggleUsageFilter(one, 'model', '  ')).toBe(one);

		expect(hasUsageFilters(empty)).toBe(false);
		expect(usageFilterCount(two)).toBe(2);
		expect(clearUsageFilters(two)).toEqual(filters());
		expect(clearUsageFilters(empty)).toBe(empty);
	});

	it('offers each key of the current view once, in the report ranking order', () => {
		const report = usageReport({
			breakdown: {
				adapter: [usageAxisEntry({ key: 'opencode' })],
				model: [
					usageAxisEntry({ key: 'big-model' }),
					usageAxisEntry({ key: 'small-model' }),
					usageAxisEntry({ key: 'big-model', label: 'alias' })
				],
				bee: [usageAxisEntry({ key: 'builder' })],
				purpose: [usageAxisEntry({ key: 'work' })]
			}
		});

		expect(usageFilterValues(report, 'model')).toEqual(['big-model', 'small-model']);
		expect(usageFilterValues(report, 'adapter')).toEqual(['opencode']);
	});
});
