// The component catalog exists only in the dev server (make web-dev): the import sits in a
// branch the build drops, so neither the page nor its code reaches dist or the binary.
import { error } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => {
	if (import.meta.env.DEV) {
		return { page: (await import('#lib/design-system/DesignSystem.svelte')).default };
	}

	error(404, 'Not found');
};
