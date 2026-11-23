import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// Admins only: the server answers anyone else 403, which shows the no-access page.
export const load: PageLoad = async () => {
	const [defaults, storage] = await Promise.all([
		get('get_project_defaults'),
		get('get_storage')
	]).catch(pageError);

	return { defaults, storage };
};
