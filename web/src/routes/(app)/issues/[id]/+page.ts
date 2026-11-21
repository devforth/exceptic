import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// ?event= picks one of the issue's events; the latest by default.
export const load: PageLoad = async ({ params, url }) => {
	const details = await get('get_issue_details', {
		issue_id: params.id,
		event_id: url.searchParams.get('event') ?? undefined
	}).catch(pageError);

	return { details };
};
