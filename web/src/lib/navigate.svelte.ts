// Moving a list to another address of its own: its filters, a page size.
import { goto } from '$app/navigation';
import { page } from '$app/state';
import { changed, type Change } from './filters.ts';

/** How long a list waits after its filters last changed before it loads them, all in one request. */
export const editPause = 1000;

type Options = {
	/** What the controls show meanwhile, by name, when it is not the change itself (a status). */
	shown?: Record<string, unknown>;
	/** Typed text: the request replaces the history entry instead of adding one. */
	typing?: boolean;
	/** Load at once (Enter in a search field). */
	now?: boolean;
};

type Address = { pathname: string; search: string };

let pending: { address: string; change: Change; typing: boolean } | undefined;
let timer: ReturnType<typeof setTimeout> | undefined;
let generation = 0;
// The values of the controls changed and not loaded yet, so they answer at once, on their page only.
const edits: Record<string, unknown> = $state({});
let editsPath = '';

const addressOf = (url: Address) => url.pathname + url.search;

function forget() {
	for (const name of Object.keys(edits)) delete edits[name];
}

function flush() {
	clearTimeout(timer);

	const edit = pending;

	pending = undefined;

	// Another navigation (a filter chip, the back button, another page) went first: it wins.
	if (!edit || addressOf(page.url) !== edit.address) {
		forget();

		return;
	}

	const target = changed(page.url, edit.change);

	if (target === addressOf(page.url)) {
		forget();

		return;
	}

	const current = ++generation;

	// eslint-disable-next-line svelte/no-navigation-without-resolve -- this page with another query
	void goto(target, { reset: false, replace: edit.typing }).finally(() => {
		if (current === generation && !pending) forget();
	});
}

/**
 * Changes the list's filters: the controls show the change at once, and the list loads once
 * nothing has changed for editPause, every change made meanwhile in the same request.
 */
export function showChanged(url: Address, change: Change, options: Options = {}): void {
	// Text goes trimmed; the field keeps what was typed, spaces included.
	const trimmed = Object.fromEntries(
		Object.entries(change).map(([name, value]) => [
			name,
			typeof value === 'string' ? value.trim() : value
		])
	);

	pending = {
		address: pending?.address ?? addressOf(url),
		change: { ...pending?.change, ...trimmed },
		typing: (options.typing ?? false) && (pending?.typing ?? true)
	};
	Object.assign(edits, options.shown ?? change);
	editsPath = url.pathname;
	clearTimeout(timer);

	if (options.now) {
		flush();
	} else {
		timer = setTimeout(flush, editPause);
	}
}

/** What a control shows: its change not loaded yet, else the list's value. */
export function shown<T>(name: string, current: T): T {
	return name in edits && page.url.pathname === editsPath ? (edits[name] as T) : current;
}

/** Whether changes wait to be loaded. */
export function editing(): boolean {
	return Object.keys(edits).length > 0 && page.url.pathname === editsPath;
}
