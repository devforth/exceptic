// The tabs of Tabs.svelte (a .ts module, so pages and eslint resolve the type); their keys are
// #lib/roving.ts.
export type Tab = { id: string; label: string };

/** A segmented control, or names over a line with the chosen one underlined. */
export type TabsVariant = 'segmented' | 'underline';
