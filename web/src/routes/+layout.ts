import { get, pageError } from '#lib/api/client.ts';
import { oldUI } from '#lib/nav.ts';
import type { LayoutLoad } from './$types';

// A single-page app: the Go server sends the same page for every address, the browser renders it.
export const ssr = false;

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
