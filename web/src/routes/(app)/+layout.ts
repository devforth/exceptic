import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { get, pageError } from '#lib/api/client.ts';
import type { LayoutLoad } from './$types';

/** Who is logged in, for the menu; read again after each navigation (app:session). */
export const load: LayoutLoad = async ({ depends, route }) => {
	depends('app:session');

	const session = await get('get_session').catch(pageError);

	// A temporary password: only the account page is open, to choose one.
	if (session.user.must_change_password && route.id !== '/(app)/account') {
		redirect(307, resolve('/(app)/account'));
	}

	return { session };
};
