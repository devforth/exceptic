import { get, listAll, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
	const [projects, storage] = await Promise.all([
		listAll('list_projects'),
		get('get_storage')
	]).catch(pageError);

	return { projects, storage };
};
