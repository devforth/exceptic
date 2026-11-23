import { describe, expect, it, vi } from 'vitest';
import { columnSort } from '#lib/components/table.ts';
import {
	columnSorts,
	issueChips,
	issueInput,
	readIssueFilters,
	showsResolved,
	sortedBy,
	withStatus
} from './issues.ts';

const projects = [
	{ id: 1, name: 'backend' },
	{ id: 2, name: 'frontend' }
];

const address = (query: string) => new URL(`http://host/issues?${query}`);
const filtersOf = (query: string) => readIssueFilters(address(query).searchParams, projects);

describe('the issue list address', () => {
	it('keeps known values only, with the defaults', () => {
		expect(
			filtersOf(
				'project=1&project=ghost&level=error&level=warn&status=hack&sort=drop&from=2026-11-10&to=2026-11-01&query=%20x%20'
			)
		).toEqual({
			project: ['backend'],
			level: ['error'],
			status: 'unresolved',
			query: 'x',
			from: '2026-11-01',
			to: '2026-11-10',
			sort: 'last_seen'
		});
	});

	it('asks for the whole last day', () => {
		expect(issueInput(filtersOf('status=all&to=2026-11-10'))).toMatchObject({
			status: 'all',
			until: '2026-11-11T00:00:00.000Z'
		});
	});

	it('writes the default sort and status as no parameter, from the first page', () => {
		const url = address('sort=event_count&page=3&per_page=50');

		expect(sortedBy(url, 'last_seen')).toBe('/issues?per_page=50');
		expect(sortedBy(url, 'project')).toBe('/issues?per_page=50&sort=project');
		expect(withStatus(address('status=resolved'), 'unresolved')).toBe('/issues');
		expect(withStatus(address(''), 'all')).toBe('/issues?status=all');
	});

	it('sorts a column its natural way first, then the reverse', () => {
		expect(columnSort(columnSorts.lastSeen, 'last_seen')).toEqual({
			direction: 'descending',
			next: 'last_seen_asc'
		});
		expect(columnSort(columnSorts.project, 'last_seen')).toEqual({ next: 'project' });
		expect(columnSort(columnSorts.events, 'last_seen').next).toBe('event_count');
	});

	it('shows a Resolved column where resolved issues are listed', () => {
		expect(showsResolved('resolved')).toBe(true);
		expect(showsResolved('all')).toBe(true);
		expect(showsResolved('unresolved')).toBe(false);
	});
});

describe('the chips of the issue list', () => {
	it('are none for the defaults', () => {
		const url = address('status=unresolved&sort=last_seen');

		expect(issueChips(url, filtersOf(url.search.slice(1)))).toEqual([]);
	});

	it('remove one value each, from the first page', () => {
		vi.useFakeTimers({ toFake: ['Date'] });
		vi.setSystemTime(new Date('2026-11-23T12:00:00Z'));

		const url = address(
			'project=backend&project=frontend&status=resolved&level=error&sort=event_count&from=2026-11-01&per_page=50&page=2'
		);
		const chips = issueChips(url, filtersOf(url.search.slice(1)));

		expect(chips.map((chip) => chip.label)).toEqual([
			'Project',
			'Status',
			'Level',
			'Dates',
			'Sort'
		]);
		expect(chips[0].values[0]).toEqual({
			text: 'backend',
			kind: 'project',
			remove:
				'/issues?status=resolved&level=error&sort=event_count&from=2026-11-01&per_page=50&project=frontend'
		});
		expect(chips[1].values[0]).toMatchObject({ text: 'resolved', kind: 'status' });
		expect(chips[3].values[0].text).toBe('from 1 Nov');
		vi.useRealTimers();
		expect(chips[4].values[0]).toEqual({
			text: 'most events',
			remove:
				'/issues?project=backend&project=frontend&status=resolved&level=error&from=2026-11-01&per_page=50'
		});
	});
});
