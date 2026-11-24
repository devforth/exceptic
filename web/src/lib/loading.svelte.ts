// Whether a list page is loading again: its filters changed, or it loads another address of its own.
import { navigating, page } from '$app/state';
import { editing } from './navigate.svelte.ts';

/** True while this page's filters wait or load; read it inside $derived or markup. */
export function reloading(): boolean {
	return editing() || (navigating.to !== null && navigating.to.route.id === page.route.id);
}
