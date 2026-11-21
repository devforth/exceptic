import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

/** How to connect an agent, and the connected apps of ?user= (admins; someone they may not manage is a 404). */
export const load: PageLoad = async ({ url }) => {
	const owner = url.searchParams.get('user');
	const [setup, agents] = await Promise.all([
		get('get_mcp_setup'),
		get('list_agents', owner ? { user_id: owner } : {})
	]).catch(pageError);

	return { setup, agents };
};
