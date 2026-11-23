// The chips of the filters a list applies (ActiveFilters.svelte), and their builders: each
// value's chip links to the list's address without it, from the first page.
import { changed, today } from '#lib/filters.ts';
import { rangeText } from './dates.ts';

export type FilterValue = {
	text: string;
	/** Drawn as a badge of this kind, or as plain text. */
	kind?: 'project' | 'level' | 'status';
	/** The address without this value. */
	remove: string;
};

export type FilterChip = { label: string; values: FilterValue[] };

/** An address: a URL, or SvelteKit's page.url. */
type Address = { pathname: string; search: string };

/** A filter of several values (?project=a&project=b): each value removable on its own. */
export function listChip(
	url: Address,
	label: string,
	name: string,
	values: readonly string[],
	kind?: FilterValue['kind']
): FilterChip {
	return {
		label,
		values: values.map((value) => ({
			text: value,
			...(kind ? { kind } : {}),
			remove: changed(url, { [name]: values.filter((other) => other !== value) })
		}))
	};
}

/** A text filter (?query=…): its text, removed with its parameter. */
export function textChip(url: Address, label: string, name: string, value: string): FilterChip {
	return { label, values: value ? [{ text: value, remove: changed(url, { [name]: null }) }] : [] };
}

/** The days of a DateRange (?from=…&to=…), removed together. */
export function daysChip(url: Address, from: string, to: string): FilterChip {
	return {
		label: 'Dates',
		values:
			from || to
				? [{ text: rangeText(from, to, today()), remove: changed(url, { from: null, to: null }) }]
				: []
	};
}

/** The chips that have a value: the filters applied. */
export function appliedChips(chips: FilterChip[]): FilterChip[] {
	return chips.filter((chip) => chip.values.length > 0);
}
