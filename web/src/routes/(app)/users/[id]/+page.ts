import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { get, listAll, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// The projects offered are the viewer's. An id of another shape and an unknown user get Not found
// from get_user, other roles the no-access page.
export const load: PageLoad = async ({ params, parent }) => {
	const [managed, projects, { session }] = await Promise.all([
		get('get_user', { user_id: params.id }).catch(pageError),
		listAll('list_projects').catch(pageError),
		parent()
	]);

	// One's own name and password are changed on the account page.
	if (managed.user.id === session.user.id) {
		redirect(307, resolve('/(app)/account'));
	}

	return { managed, projects };
};
