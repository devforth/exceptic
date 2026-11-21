import { get, listAll, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => {
	const [connection, defaults, projects] = await Promise.all([
		get('get_connection', { connection_id: params.id }),
		get('get_connection_defaults'),
		listAll('list_projects')
	]).catch(pageError);

	return { connection, defaults, projects };
};
