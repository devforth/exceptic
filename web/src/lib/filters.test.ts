import { describe, expect, it } from 'vitest';
import {
	changed,
	cleared,
	dayRange,
	readChoice,
	readChoices,
	readDays,
	readPaging,
	readProjects,
	readText
} from './filters.ts';

const params = (query: string) => new URLSearchParams(query);

describe('reading a list address', () => {
	it('keeps only known values, in the known order', () => {
		expect(
			readChoices(params('level=warn&level=bogus&level=error,fatal'), 'level', [
				'fatal',
				'error',
				'warn'
			])
		).toEqual(['fatal', 'error', 'warn']);
	});

	it('reads projects by name or id, as names', () => {
		const projects = [
			{ id: 1, name: 'backend' },
			{ id: 2, name: 'frontend' },
			{ id: 3, name: 'worker' }
		];

		expect(readProjects(params('project=frontend&project=1&project=ghost'), projects)).toEqual([
			'backend',
			'frontend'
		]);
	});

	it('reads one known value or none', () => {
		expect(readChoice(params('sort=event_count'), 'sort', ['event_count', 'project'])).toBe(
			'event_count'
		);
		expect(readChoice(params('sort=drop'), 'sort', ['event_count'])).toBe('');
		expect(readChoice(params(''), 'sort', ['event_count'])).toBe('');
	});

	it('reads paging with its defaults', () => {
		expect(readPaging(params('page=3&per_page=50'))).toEqual({ page: 3, perPage: 50 });
		expect(readPaging(params('page=-1&per_page=7'))).toEqual({ page: 1, perPage: 20 });
	});

	it('caps the search and swaps a reversed range of days', () => {
		expect(readText(params(`query=${'x'.repeat(300)}`), 'query')).toHaveLength(200);
		expect(readDays(params('from=2026-11-10&to=2026-11-01'))).toEqual({
			from: '2026-11-01',
			to: '2026-11-10'
		});
		expect(readDays(params('from=nope'))).toEqual({ from: '', to: '' });
		expect(dayRange('2026-11-01', '2026-11-10')).toEqual({
			since: '2026-11-01T00:00:00Z',
			until: '2026-11-11T00:00:00.000Z'
		});
	});
});

describe('changing it', () => {
	const url = new URL('http://host/logs?project=a&level=error&page=3&per_page=50');

	it('starts a changed filter from the first page', () => {
		expect(changed(url, { level: ['warn', 'info'] })).toBe(
			'/logs?project=a&per_page=50&level=warn&level=info'
		);
		expect(changed(url, { project: null })).toBe('/logs?level=error&per_page=50');
	});

	it('changes only the page when asked', () => {
		expect(changed(url, { page: '4' })).toBe('/logs?project=a&level=error&per_page=50&page=4');
	});

	it('clears every filter but the page size', () => {
		expect(cleared(url)).toBe('/logs?per_page=50');
	});
});
