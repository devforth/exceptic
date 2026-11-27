import { levels } from '#lib/api/catalog.gen.ts';
import { get, listAll, pageError } from '#lib/api/client.ts';
import {
	dayRange,
	offered,
	pageWithin,
	readChoices,
	readDays,
	readPaging,
	readProjects,
	readText
} from '#lib/filters.ts';
import type { PageLoad } from './$types';
import { listPage, stoppedList } from './logs.ts';

export const load: PageLoad = async ({ url }) => {
	const params = url.searchParams;
	// The filters offer the projects and levels some log has, and whatever the address chose.
	const [projects, present, withLogs] = await Promise.all([
		listAll('list_projects'),
		get('list_log_levels'),
		get('list_log_projects')
	]).catch(pageError);
	const filters = {
		project: readProjects(params, projects),
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
	const page = pageWithin(paging, count.count);
	// The list reads only what the count found: none when it found nothing.
	const listed = listPage(count, page, paging.perPage);
	const logs = listed
		? await get('list_logs', { ...input, ...listed }).catch(pageError)
		: undefined;

	return {
		filters,
		projectNames: offered(
			projects.map((project) => project.name),
			withLogs.projects,
			filters.project
		),
		levelChoices: offered(levels.log, present.levels, filters.level),
		count,
		page,
		perPage: paging.perPage,
		logs: logs?.items ?? [],
		listStopped: stoppedList(logs)
	};
};
