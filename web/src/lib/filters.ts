// The filters of a list in its address: ?project=a&project=b&level=error&query=…&from=…&to=…
// &page=2&per_page=50. Only known values are kept, so an address edited by hand still works;
// a change of a filter starts again from the first page.

export const perPageChoices = [20, 50, 100] as const;
const defaultPerPage = 20;
const maxSearch = 200;

export type Paging = { page: number; perPage: number };

export function readPaging(params: URLSearchParams): Paging {
	const page = Number.parseInt(params.get('page') ?? '', 10);
	const perPage = Number.parseInt(params.get('per_page') ?? '', 10);

	return {
		page: Number.isFinite(page) && page > 1 ? page : 1,
		perPage: (perPageChoices as readonly number[]).includes(perPage) ? perPage : defaultPerPage
	};
}

/** The values of a list parameter that are among the known ones, in the known order. */
export function readChoices<Value extends string>(
	params: URLSearchParams,
	name: string,
	known: readonly Value[]
): Value[] {
	const given = params.getAll(name).flatMap((value) => value.split(','));

	return known.filter((value) => given.includes(value));
}

/** A text parameter, trimmed and capped. */
export function readText(params: URLSearchParams, name: string): string {
	return (params.get(name) ?? '').trim().slice(0, maxSearch);
}

const isDay = (value: string) =>
	/^\d{4}-\d{2}-\d{2}$/.test(value) && !Number.isNaN(Date.parse(value));

/** UTC days, both included; a reversed range is swapped. */
export function readDays(params: URLSearchParams): { from: string; to: string } {
	let from = params.get('from') ?? '';
	let to = params.get('to') ?? '';

	from = isDay(from) ? from : '';
	to = isDay(to) ? to : '';

	return from && to && from > to ? { from: to, to: from } : { from, to };
}

/** The days as the times the API filters on: since the first day, until after the last. */
export function dayRange(from: string, to: string): { since?: string; until?: string } {
	const range: { since?: string; until?: string } = {};

	if (from) range.since = `${from}T00:00:00Z`;
	if (to) range.until = new Date(Date.parse(`${to}T00:00:00Z`) + 86_400_000).toISOString();

	return range;
}

/** Today in UTC, the last day a date field offers. */
export function today(): string {
	return new Date().toISOString().slice(0, 10);
}

export type Change = Record<string, string | readonly string[] | null>;

/** An address: a URL, or SvelteKit's page.url. */
type Address = { pathname: string; search: string };

/**
 * The address with some parameters changed (null or empty removes one). Unless only the
 * page changes, it starts again from the first page.
 */
export function changed(url: Address, change: Change): string {
	const params = new URLSearchParams(url.search);

	if (!('page' in change)) {
		params.delete('page');
	}

	for (const [name, value] of Object.entries(change)) {
		params.delete(name);

		for (const item of value === null ? [] : typeof value === 'string' ? [value] : value) {
			if (item !== '') params.append(name, item);
		}
	}

	const query = params.toString();

	return url.pathname + (query ? `?${query}` : '');
}

/** Every filter removed; the page size stays. */
export function cleared(url: Address): string {
	const perPage = new URLSearchParams(url.search).get('per_page');

	return url.pathname + (perPage ? `?per_page=${encodeURIComponent(perPage)}` : '');
}
