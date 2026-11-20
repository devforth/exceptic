// The columns of Table.svelte (a .ts module, so pages and eslint resolve the type).
export type TableColumn<Key extends string = string> = {
	key: Key;
	label: string;
	/** Right-aligned and on one line. */
	number?: boolean;
	/** Hidden on narrow screens: what a phone can do without. */
	wideOnly?: boolean;
	/** The row's buttons: as narrow as they are. */
	actions?: boolean;
};
