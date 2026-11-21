import { error } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// Admins only, like the list: other roles get Not found (the server refuses create_user anyway).
export const load: PageLoad = async ({ parent }) => {
	const { session } = await parent();

	if (!session.can_administer) {
		error(404, 'not found');
	}
};
