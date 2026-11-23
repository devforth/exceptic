// Moving a list to another address of its own: a filter, a page size.
import { goto } from '$app/navigation';
import { changed, type Change } from './filters.ts';

/** An address: a URL, or SvelteKit's page.url. */
type Address = { pathname: string; search: string };

/** Shows the list with the change; typing replaces the history entry instead of adding one. */
export function showChanged(url: Address, change: Change, replace = false): void {
	// eslint-disable-next-line svelte/no-navigation-without-resolve -- this page with another query
	void goto(changed(url, change), { reset: false, replace });
}
