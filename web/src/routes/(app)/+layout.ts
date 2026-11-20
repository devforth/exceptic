import { get, pageError } from '#lib/api/client.ts';
import { oldUI } from '#lib/nav.ts';
import type { LayoutLoad } from './$types';

/** Who is logged in, for the menu; read again after each navigation (app:session). */
export const load: LayoutLoad = async ({ depends }) => {
	depends('app:session');

	const session = await get('get_session').catch(pageError);

	// A temporary password: only the account page is open, and it is still the old UI's.
	if (session.user.must_change_password) {
		location.replace(oldUI.account);

		return new Promise<never>(() => {});
	}

	return { session };
};
