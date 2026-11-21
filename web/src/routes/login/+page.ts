import { pageError, send } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

/** The login has no session yet: what it shows comes from login_state. */
export const load: PageLoad = async ({ url }) => {
	const state = await send('login_state', { next: url.searchParams.get('next') ?? '' }).catch(
		pageError
	);

	// Logged in already: on to the page asked for.
	if (state.redirect) {
		location.replace(state.redirect);

		return new Promise<never>(() => {});
	}

	return { state };
};
