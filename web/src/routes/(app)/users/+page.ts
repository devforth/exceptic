import { listAll, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// Admins only: for other roles list_users answers 404, the page Not found.
export const load: PageLoad = async () => {
	const users = await listAll('list_users').catch(pageError);

	return { users };
};
