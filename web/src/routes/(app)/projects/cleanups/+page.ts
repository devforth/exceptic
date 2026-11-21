import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
	const history = await get('list_cleanups').catch(pageError);

	return { history };
};
