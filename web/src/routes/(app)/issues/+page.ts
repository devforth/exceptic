import { levels } from '#lib/api/catalog.gen.ts';
import { get, listAll, pageError } from '#lib/api/client.ts';
import { offeredLevels, pageWithin, readPaging } from '#lib/filters.ts';
import type { PageLoad } from './$types';
import { issueInput, readIssueFilters } from './issues.ts';

export const load: PageLoad = async ({ url }) => {
	const params = url.searchParams;
	const [projects, present] = await Promise.all([
		listAll('list_projects'),
		get('list_issue_levels')
	]).catch(pageError);
	const filters = readIssueFilters(params, projects);
	const input = issueInput(filters);
	const paging = readPaging(params);
	const count = await get('count_issues', input).catch(pageError);
	const page = pageWithin(paging, count.count);
	// The cursor of list_issues is an offset.
	const issues = await get('list_issues', {
		...input,
		sort: filters.sort,
		limit: paging.perPage,
		cursor: String((page - 1) * paging.perPage)
	}).catch(pageError);

	return {
		filters,
		projectNames: projects.map((project) => project.name),
		levelChoices: offeredLevels(levels.event, present.levels, filters.level),
		count: count.count,
		page,
		perPage: paging.perPage,
		issues: issues.items ?? []
	};
};
