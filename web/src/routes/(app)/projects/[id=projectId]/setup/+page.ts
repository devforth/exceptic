import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

/** Read again while the page waits for the project's first data (app:project-setup). */
export const load: PageLoad = async ({ params, depends }) => {
	depends('app:project-setup');

	const setup = await get('get_project_setup', { project: params.id }).catch(pageError);

	return { setup };
};
