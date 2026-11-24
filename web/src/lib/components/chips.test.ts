import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appliedChips, daysChip, listChip, textChip } from './chips.ts';

const url = { pathname: '/logs', search: '?project=backend&project=frontend&query=boom&page=2' };

// Days of this year read without it, as the date range field shows them.
beforeEach(() => {
	vi.useFakeTimers({ toFake: ['Date'] });
	vi.setSystemTime(new Date('2026-11-23T12:00:00Z'));
});

afterEach(() => {
	vi.useRealTimers();
});

describe('the chip builders', () => {
	it('remove each value of a list filter on its own, or the whole filter, from the first page', () => {
		expect(listChip(url, 'Project', 'project', ['backend', 'frontend'], 'project')).toEqual({
			label: 'Project',
			remove: '/logs?query=boom',
			values: [
				{ text: 'backend', kind: 'project', remove: '/logs?query=boom&project=frontend' },
				{ text: 'frontend', kind: 'project', remove: '/logs?query=boom&project=backend' }
			]
		});
	});

	it('draw a value without a kind as text', () => {
		expect(listChip(url, 'Level', 'level', ['error']).values[0]).toEqual({
			text: 'error',
			remove: '/logs?project=backend&project=frontend&query=boom'
		});
	});

	it('remove a text filter with its parameter', () => {
		expect(textChip(url, 'Search', 'query', 'boom')).toEqual({
			label: 'Search',
			remove: '/logs?project=backend&project=frontend',
			values: [{ text: 'boom', remove: '/logs?project=backend&project=frontend' }]
		});
		expect(textChip(url, 'Search', 'query', '').values).toEqual([]);
	});

	it('remove both days of a range together', () => {
		const ranged = { pathname: '/issues', search: '?from=2026-11-01&to=2026-11-10' };

		expect(daysChip(ranged, '2026-11-01', '2026-11-10')).toEqual({
			label: 'Dates',
			remove: '/issues',
			values: [{ text: '1 Nov – 10 Nov', remove: '/issues' }]
		});
		expect(daysChip(ranged, '', '').values).toEqual([]);
	});

	it('keep only the chips with a value', () => {
		const chips = appliedChips([
			listChip(url, 'Project', 'project', ['backend'], 'project'),
			textChip(url, 'Search', 'query', ''),
			daysChip(url, '', '')
		]);

		expect(chips.map((chip) => chip.label)).toEqual(['Project']);
	});
});
