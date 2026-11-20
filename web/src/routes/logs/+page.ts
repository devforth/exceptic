import { levels } from '#lib/api/catalog.gen.ts';
import { get, listAll, pageError } from '#lib/api/client.ts';
import { dayRange, readChoices, readDays, readPaging, readText } from '#lib/filters.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ url }) => {
	const params = url.searchParams;
	const [projects, present] = await Promise.all([
		listAll('list_projects'),
		get('list_log_levels')
	]).catch(pageError);
	const projectNames = projects.map((project) => project.name);
	const filters = {
		project: readChoices(params, 'project', projectNames),
		level: readChoices(params, 'level', levels.log),
		query: readText(params, 'query'),
		traceId: readText(params, 'trace_id'),
		...readDays(params)
	};
	const input = {
		project: filters.project,
		level: filters.level,
		query: filters.query || undefined,
		trace_id: filters.traceId || undefined,
		...dayRange(filters.from, filters.to)
	};
	const paging = readPaging(params);
	const count = await get('count_logs', input).catch(pageError);
	// A page past the end shows the last one.
	const page = Math.min(paging.page, Math.max(1, Math.ceil(count.count / paging.perPage)));
	const logs = await get('list_logs', {
		...input,
		limit: paging.perPage,
		offset: (page - 1) * paging.perPage
	}).catch(pageError);

	return {
		filters,
		projectNames,
		// The levels some log has, and the chosen ones.
		levelChoices: levels.log.filter(
			(level) => (present.levels ?? []).includes(level) || filters.level.includes(level)
		),
		count,
		page,
		perPage: paging.perPage,
		logs: logs.items ?? []
	};
};
