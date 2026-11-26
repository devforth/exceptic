import { get, pageError } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
	const [defaults, storage] = await Promise.all([
		get('get_project_defaults'),
		get('get_storage')
	]).catch(pageError);

	return { defaults, storage };
};
