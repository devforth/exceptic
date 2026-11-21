import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// Every connection comes in one page.
export const load: PageLoad = async () => {
	const connections = await get('list_connections').catch(pageError);

	return { connections: connections.items ?? [] };
};
