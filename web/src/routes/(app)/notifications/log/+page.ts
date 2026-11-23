import { choices } from '#lib/api/catalog.gen.ts';
import { get, pageError } from '#lib/api/client.ts';
import { pageWithin, readChoice, readPaging } from '#lib/filters.ts';
import type { PageLoad } from './$types';

// ?connection=<id>&status=…&page=…&per_page=…: only a known connection and status are kept.
export const load: PageLoad = async ({ url }) => {
	const params = url.searchParams;
	const connections = ((await get('list_connections').catch(pageError)).items ?? []).map(
		(connection) => ({ id: connection.id, name: connection.name })
	);
	const filters = {
		connection: readChoice(
			params,
			'connection',
			connections.map((connection) => connection.id)
		),
		status: readChoice(params, 'status', choices.list_deliveries.status)
	};
	const input = {
		connection_id: filters.connection || undefined,
		status: filters.status || undefined
	};
	const paging = readPaging(params);
	const count = await get('count_deliveries', input).catch(pageError);
	// A page past the end shows the last one.
	const page = pageWithin(paging, count.count);
	const deliveries = await get('list_deliveries', {
		...input,
		limit: paging.perPage,
		cursor: String((page - 1) * paging.perPage)
	}).catch(pageError);

	return {
		connections,
		filters,
		count: count.count,
		page,
		perPage: paging.perPage,
		deliveries: deliveries.items ?? []
	};
};
