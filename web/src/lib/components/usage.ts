// The segments of UsageBar.svelte (a .ts module, so pages and eslint resolve the type).
export type UsageSegment = {
	/** Its tooltip, e.g. "backend: 250 KB". */
	label: string;
	/** Its width, in percent of the bar. */
	percent: number;
	/** A project's color, #rrggbb; grey (the rest) without. */
	color?: string;
};
