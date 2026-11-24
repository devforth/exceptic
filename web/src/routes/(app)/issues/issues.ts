// The issue list's address: its filters, the sort of its column headings and the chips of
// what it applies. The status defaults to unresolved and the sort to last seen; neither is
// then written in the address nor shown as a chip.
import { choices, levels } from '#lib/api/catalog.gen.ts';
import {
	appliedChips,
	daysChip,
	listChip,
	textChip,
	type FilterChip
} from '#lib/components/chips.ts';
import type { ColumnSort } from '#lib/components/table.ts';
import {
	changed,
	type Change,
	dayRange,
	readChoice,
	readChoices,
	readDays,
	readProjects,
	readText
} from '#lib/filters.ts';

export type IssueStatusFilter = (typeof choices.list_issues.status)[number];
export type IssueSort = (typeof choices.list_issues.sort)[number];

export const defaultStatus: IssueStatusFilter = 'unresolved';
export const defaultSort: IssueSort = 'last_seen';

/** The sortable columns: the first click sorts the natural way (A to Z, newest or most first). */
export const columnSorts = {
	project: { ascending: 'project', descending: 'project_desc', first: 'ascending' },
	events: { ascending: 'event_count_asc', descending: 'event_count', first: 'descending' },
	firstSeen: { ascending: 'first_seen_asc', descending: 'first_seen', first: 'descending' },
	lastSeen: { ascending: 'last_seen_asc', descending: 'last_seen', first: 'descending' }
} as const satisfies Record<string, ColumnSort & { ascending: IssueSort; descending: IssueSort }>;

/** A sort in words, for its chip. */
const sortNames: Record<IssueSort, string> = {
	last_seen: 'last seen, newest first',
	last_seen_asc: 'last seen, oldest first',
	first_seen: 'first seen, newest first',
	first_seen_asc: 'first seen, oldest first',
	event_count: 'most events',
	event_count_asc: 'fewest events',
	project: 'project A–Z',
	project_desc: 'project Z–A'
};

export type IssueFilters = {
	project: string[];
	level: (typeof levels.event)[number][];
	status: IssueStatusFilter;
	query: string;
	from: string;
	to: string;
	sort: IssueSort;
};

export function readIssueFilters(
	params: URLSearchParams,
	projects: readonly { id: number; name: string }[]
): IssueFilters {
	return {
		project: readProjects(params, projects),
		level: readChoices(params, 'level', levels.event),
		status: readChoice(params, 'status', choices.list_issues.status) || defaultStatus,
		query: readText(params, 'query'),
		...readDays(params),
		sort: readChoice(params, 'sort', choices.list_issues.sort) || defaultSort
	};
}

/** The filters as count_issues and list_issues take them. */
export function issueInput(filters: IssueFilters) {
	return {
		project: filters.project,
		level: filters.level,
		status: filters.status,
		query: filters.query || undefined,
		...dayRange(filters.from, filters.to)
	};
}

type Address = { pathname: string; search: string };

/** The list in another order, from the first page; the default order is no parameter. */
export function sortedBy(url: Address, sort: string): string {
	return changed(url, { sort: sort === defaultSort ? null : sort });
}

/** The change to another status; the default status is no parameter. */
export function statusChange(status: string): Change {
	return { status: status === defaultStatus ? null : status };
}

/** The list of another status. */
export function withStatus(url: Address, status: string): string {
	return changed(url, statusChange(status));
}

/** The chips of the filters applied, each value removable on its own. */
export function issueChips(url: Address, filters: IssueFilters): FilterChip[] {
	return appliedChips([
		listChip(url, 'Project', 'project', filters.project, 'project'),
		{
			label: 'Status',
			remove: withStatus(url, defaultStatus),
			values:
				filters.status === defaultStatus
					? []
					: [{ text: filters.status, kind: 'status', remove: withStatus(url, defaultStatus) }]
		},
		listChip(url, 'Level', 'level', filters.level, 'level'),
		textChip(url, 'Search', 'query', filters.query),
		daysChip(url, filters.from, filters.to),
		{
			label: 'Sort',
			remove: sortedBy(url, defaultSort),
			values:
				filters.sort === defaultSort
					? []
					: [{ text: sortNames[filters.sort], remove: sortedBy(url, defaultSort) }]
		}
	]);
}

/** The Resolved column: only where resolved issues are listed. */
export function showsResolved(status: IssueStatusFilter): boolean {
	return status === 'resolved' || status === 'all';
}
