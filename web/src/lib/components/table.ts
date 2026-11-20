// The columns and options of Table.svelte (a .ts module, so pages and eslint resolve the types).
export type TableColumn<Key extends string = string> = {
	key: Key;
	label: string;
	/** Right-aligned and on one line. */
	number?: boolean;
	/** Hidden on narrow screens: what a phone can do without. */
	wideOnly?: boolean;
	/** The row's buttons: as narrow as they are. */
	actions?: boolean;
	/** The cell names its row (th scope=row): the key of a key–value table. */
	rowHeader?: boolean;
	/** Its heading sorts the list. */
	sort?: ColumnSort;
};

/** The list's sort values of a column's two directions, and the one of its first click. */
export type ColumnSort = { ascending: string; descending: string; first: SortDirection };

export type SortDirection = 'ascending' | 'descending';

/** The list's sort and the address of another sort. */
export type TableSort = { current: string; href: (sort: string) => string };

/** A row's color, by the state of what it shows (an issue's status); faded is dimmed. */
export type RowTone = 'error' | 'success' | 'faded';

/** How a column is sorted now, and the sort its heading links to: the reverse once sorted. */
export function columnSort(
	sort: ColumnSort,
	current: string
): { direction?: SortDirection; next: string } {
	if (current === sort.ascending) return { direction: 'ascending', next: sort.descending };
	if (current === sort.descending) return { direction: 'descending', next: sort.ascending };

	return { next: sort[sort.first] };
}
