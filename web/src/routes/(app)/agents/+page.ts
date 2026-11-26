import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

/** How to connect an agent, and the viewer's own connected apps. */
export const load: PageLoad = async () => {
	const [setup, agents] = await Promise.all([get('get_mcp_setup'), get('list_agents')]).catch(
		pageError
	);

	return { setup, agents };
};
