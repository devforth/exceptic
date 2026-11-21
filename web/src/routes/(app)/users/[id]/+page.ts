import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// An id of another shape, an unknown user and other roles get Not found from get_user.
export const load: PageLoad = async ({ params, parent }) => {
	const [managed, { session }] = await Promise.all([
		get('get_user', { user_id: params.id }).catch(pageError),
		parent()
	]);

	// One's own name and password are changed on the account page.
	if (managed.user.id === session.user.id) {
		redirect(307, resolve('/(app)/account'));
	}

	return { managed };
};
