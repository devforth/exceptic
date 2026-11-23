import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// Admins only: for other roles list_users answers 403, the no-access page. Every user comes in
// one page.
export const load: PageLoad = async () => {
	const users = (await get('list_users').catch(pageError)).items ?? [];

	return { users };
};
