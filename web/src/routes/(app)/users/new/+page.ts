import { error } from '@sveltejs/kit';
import { listAll, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// Admins only, like the list: other roles get the no-access page (the server refuses create_user
// anyway).
// The projects offered are the viewer's.
export const load: PageLoad = async ({ parent }) => {
	const { session } = await parent();

	if (!session.can_administer) {
		error(403, 'your role may not add users');
	}

	const projects = await listAll('list_projects').catch(pageError);

	return { projects };
};
