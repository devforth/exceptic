// The web app's requests to the catalog (/api/ui/<name>), typed by catalog.gen.ts.
import { error } from '@sveltejs/kit';
import { oldUI, withQuery } from '#lib/nav.ts';
import { apiPrefix, endpoints, maxLimit, type Endpoints } from './catalog.gen.ts';

export type Name = keyof Endpoints;
export type In<N extends Name> = Endpoints[N]['in'];
export type Out<N extends Name> = Endpoints[N]['out'];
export type Item<N extends Name> = Out<N> extends { items: (infer T)[] | null } ? T : never;

type MethodOf<N extends Name> = (typeof endpoints)[N]['method'];
/** Reads: the only requests a load function may make (SvelteKit runs loads on hover). */
export type GetName = { [N in Name]: MethodOf<N> extends 'GET' ? N : never }[Name];
/** Changes, made only by an action of the user. */
export type PostName = { [N in Name]: MethodOf<N> extends 'POST' ? N : never }[Name];
/** Paged lists: items and next_cursor. */
export type ListName = {
	[N in GetName]: Out<N> extends { items: unknown; next_cursor?: string } ? N : never;
}[GetName];

/** The input may be left out when every field is optional. */
type Args<N extends Name> = Record<never, never> extends In<N> ? [input?: In<N>] : [input: In<N>];

/** A refused request: status and the server's message. 404 also stands for "not allowed". */
export class ApiError extends Error {
	constructor(
		readonly status: number,
		message: string
	) {
		super(message);
	}
}

/** For load functions: a refused request shows the error page with its status (404: not found). */
export function pageError(failure: unknown): never {
	if (failure instanceof ApiError) {
		error(failure.status, failure.message);
	}

	throw failure;
}

export function get<N extends GetName>(name: N, ...[input]: Args<N>): Promise<Out<N>> {
	return request(name, input ?? {});
}

export function post<N extends PostName>(name: N, ...[input]: Args<N>): Promise<Out<N>> {
	return request(name, input ?? {});
}

/** Every item of a paged list, page by page. */
export async function listAll<N extends ListName>(name: N, input?: In<N>): Promise<Item<N>[]> {
	const items: Item<N>[] = [];
	let cursor: string | undefined;

	do {
		const page = (await request(name, { ...(input ?? {}), limit: maxLimit, cursor })) as {
			items: Item<N>[] | null;
			next_cursor?: string;
		};

		items.push(...(page.items ?? []));
		cursor = page.next_cursor;
	} while (cursor);

	return items;
}

async function request<N extends Name>(name: N, input: object): Promise<Out<N>> {
	const { method, text } = endpoints[name];
	const init: RequestInit =
		method === 'GET'
			? { credentials: 'same-origin' }
			: {
					method,
					credentials: 'same-origin',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(input)
				};
	const response = await fetch(apiPrefix + name + (method === 'GET' ? query(input) : ''), init);

	if (response.status === 401) {
		return toLogin();
	}

	if (!response.ok) {
		const body = (await response.json().catch(() => null)) as { error?: string } | null;

		throw new ApiError(response.status, body?.error ?? response.statusText);
	}

	return (text ? await response.text() : await response.json()) as Out<N>;
}

/** A read's input as the REST transport takes it: a list as repeated names, nothing for an empty value. */
export function query(input: object): string {
	const params = new URLSearchParams();

	for (const [name, value] of Object.entries(input) as [string, unknown][]) {
		for (const item of Array.isArray(value) ? value : [value]) {
			if (item !== undefined && item !== null) {
				params.append(name, String(item));
			}
		}
	}

	const encoded = params.toString();

	return encoded ? `?${encoded}` : '';
}

const loginRedirectKey = 'exceptic-login-redirect';
// A second 401 this soon after sending the browser to the login is an error, not another trip.
const loginLoopWindow = 10_000;

// Set once the browser is sent to the login: the other requests of the page wait with it.
let leaving: Promise<never> | undefined;

/** The session ended: the login page, then back here. Never settles, the page is leaving. */
function toLogin(): Promise<never> {
	if (leaving) {
		return leaving;
	}

	const now = Date.now();
	let last = 0;

	try {
		last = Number(sessionStorage.getItem(loginRedirectKey)) || 0;
	} catch {
		// storage blocked: no loop guard
	}

	if (now - last < loginLoopWindow) {
		throw new ApiError(401, 'login required');
	}

	try {
		sessionStorage.setItem(loginRedirectKey, String(now));
	} catch {
		// storage blocked
	}

	location.assign(
		withQuery(oldUI.login, { next: location.pathname + location.search + location.hash })
	);
	leaving = new Promise<never>(() => {});

	return leaving;
}
