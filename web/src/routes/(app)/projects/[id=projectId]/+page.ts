import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// Admins only: the server answers anyone else, and an unknown project, 404.
export const load: PageLoad = async ({ params }) => {
	const [project, storage] = await Promise.all([
		get('get_project', { project: String(params.id) }),
		get('get_storage')
	]).catch(pageError);

	return { project, storage };
};
