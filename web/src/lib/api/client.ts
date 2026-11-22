// The web app's requests to the catalog and its plain requests (/api/ui/<name>), typed by catalog.gen.ts.
import { error } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { withQuery } from '#lib/nav.ts';
import {
	apiPrefix,
	endpoints,
	maxLimit,
	plainRequests,
	type Endpoints,
	type ErrorBody,
	type PlainRequests
} from './catalog.gen.ts';

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

/** Logins and credentials, which no endpoint handles. */
export type PlainName = keyof PlainRequests;

/** The input may be left out when every field is optional. */
type Args<Input> = Record<never, never> extends Input ? [input?: Input] : [input: Input];

/**
 * A refused request: status and the server's message. 404 also stands for "not allowed".
 * A refused form names the problem of each field by its input name, and a record that
 * already does what was asked (duplicate).
 */
export class ApiError extends Error {
	readonly fields?: Record<string, string>;
	readonly duplicate?: { id: string; name: string };

	constructor(
		readonly status: number,
		message: string,
		body?: Omit<ErrorBody, 'error'>
	) {
		super(message);
		this.fields = body?.fields;
		this.duplicate = body?.duplicate ?? undefined;
	}
}

/** For load functions: a refused request shows the error page with its status (404: not found). */
export function pageError(failure: unknown): never {
	if (failure instanceof ApiError) {
		error(failure.status, failure.message);
	}

	throw failure;
}

export function get<N extends GetName>(name: N, ...[input]: Args<In<N>>): Promise<Out<N>> {
	return request(name, input ?? {});
}

export function post<N extends PostName>(name: N, ...[input]: Args<In<N>>): Promise<Out<N>> {
	return request(name, input ?? {});
}

// Their 401 answers the form (a wrong password), not an ended session.
const loginRequests: readonly PlainName[] = ['login', 'login_state'];

/** A plain request: a login or a credential. */
export function send<N extends PlainName>(
	name: N,
	...[input]: Args<PlainRequests[N]['in']>
): Promise<PlainRequests[N]['out']> {
	return call(name, plainRequests[name].method, input ?? {}, loginRequests.includes(name));
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

function request<N extends Name>(name: N, input: object): Promise<Out<N>> {
	return call(name, endpoints[name].method, input, false);
}

/** loginAnswer: a 401 is the answer to show, not a reason to go to the login. */
async function call<Output>(
	name: string,
	method: 'GET' | 'POST',
	input: object,
	loginAnswer: boolean
): Promise<Output> {
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

	if (response.status === 401 && !loginAnswer) {
		return toLogin();
	}

	if (!response.ok) {
		const body = (await response.json().catch(() => null)) as Partial<ErrorBody> | null;

		throw new ApiError(response.status, body?.error ?? response.statusText, body ?? undefined);
	}

	if (response.status === 204) {
		return undefined as Output;
	}

	return (await response.json()) as Output;
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
		withQuery(resolve('/login'), { next: location.pathname + location.search + location.hash })
	);
	leaving = new Promise<never>(() => {});

	return leaving;
}
