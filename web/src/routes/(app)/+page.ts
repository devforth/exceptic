import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

/** The start page is the issue list. */
export const load: PageLoad = () => {
	redirect(307, resolve('/(app)/issues'));
};
