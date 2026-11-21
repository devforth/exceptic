import { ApiError, pageError, send } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

/**
 * An agent's sign-in: the server checks its request (the query as the agent sent it) and gives
 * the form token of this session for exactly this request. A refused request shows its reason;
 * the browser is never sent back to an unchecked redirect_uri.
 */
export const load: PageLoad = async ({ url }) => {
	const query = url.search.slice(1);

	try {
		return { query, request: await send('authorization_request', { query }) };
	} catch (failure) {
		if (failure instanceof ApiError && failure.status === 400) {
			return { query, refused: failure.message };
		}

		return pageError(failure);
	}
};
