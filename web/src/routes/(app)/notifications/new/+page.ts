import { get, listAll, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
	const [defaults, projects] = await Promise.all([
		get('get_connection_defaults'),
		listAll('list_projects')
	]).catch(pageError);

	return { defaults, projects };
};
