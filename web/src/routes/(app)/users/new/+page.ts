import { listAll, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

// The projects offered are the viewer's.
export const load: PageLoad = async () => {
	const projects = await listAll('list_projects').catch(pageError);

	return { projects };
};
