// The chips of the filters a list applies (ActiveFilters.svelte).
export type FilterValue = {
	text: string;
	/** Drawn as a badge of this kind, or as plain text. */
	kind?: 'project' | 'level';
	/** The address without this value. */
	remove: string;
};

export type FilterChip = { label: string; values: FilterValue[] };
