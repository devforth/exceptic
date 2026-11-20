// The look of every form control: one source, so they line up wherever they sit together.

/** The frame of a field. */
export const fieldClasses =
	'h-9 rounded-control border border-border px-2.5 text-text focus-visible:border-accent focus-visible:outline-none';

/** Text and date fields, selects, the button of a Picker. */
export const controlClasses = `${fieldClasses} bg-surface text-sm`;
