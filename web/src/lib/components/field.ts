// The look of every form control: one source, so they line up wherever they sit together.

/** Keyboard focus of a control: an accent border and a soft ring around it (an outline only in
 * forced colors, where shadows are dropped). */
export const focusGlow =
	'focus-visible:border-accent focus-visible:shadow-[0_0_0_3px_var(--color-focus)] focus-visible:outline-hidden';

/** A refused value (aria-invalid): a red border and ring. */
const invalidGlow =
	'aria-invalid:border-error aria-invalid:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-error)_20%,transparent)] aria-invalid:focus-visible:border-error';

/** The frame of a field, without its height. */
export const fieldFrame = `rounded-control border border-border px-2.5 text-text ${focusGlow} ${invalidGlow}`;

/** The frame of a field. */
export const fieldClasses = `h-9 ${fieldFrame}`;

/** The one large, centered field of a page (TextField's hero). */
export const heroFieldClasses = `w-full rounded-popup border border-border px-4 py-3 text-center text-lg text-text ${focusGlow} ${invalidGlow}`;

/** A hovered dropdown control (a Select's button, a Picker's): a muted border and a soft ring of
 * it; keyboard focus and an open popup keep their accent look. */
export const controlHoverClasses =
	'hover:border-muted hover:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-muted)_22%,transparent)]';

/** Text and date fields, selects, the button of a Picker. */
export const controlClasses = `${fieldClasses} bg-surface text-sm`;

const checkBase =
	'relative m-0 size-[18px] shrink-0 cursor-pointer appearance-none border-[1.5px] border-muted bg-surface p-0 align-[-4px] transition-[background-color,border-color,box-shadow] duration-150 not-checked:not-disabled:hover:border-accent checked:border-accent checked:bg-accent focus-visible:shadow-[0_0_0_3px_var(--color-focus)] focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-45 motion-reduce:transition-none after:absolute after:opacity-0 after:transition-[scale,opacity] after:duration-150 checked:after:scale-100 checked:after:opacity-100 motion-reduce:after:transition-none forced-colors:border-[CanvasText] forced-colors:bg-[Canvas] forced-colors:forced-color-adjust-none forced-colors:checked:border-[Highlight] forced-colors:checked:bg-[Highlight] forced-colors:disabled:border-[GrayText] forced-colors:disabled:opacity-100';

/** A checkbox and a radio button: one look everywhere (Choice, the rows of a Table, a Picker).
 * A checkbox may be indeterminate (some of a list). */
export const checkClasses = {
	checkbox: `${checkBase} rounded-[5px] after:box-content after:top-[1.5px] after:left-[5px] after:h-[9px] after:w-1 after:scale-40 after:rotate-45 after:border-r-2 after:border-b-2 after:border-accent-text indeterminate:border-accent indeterminate:bg-accent indeterminate:after:top-[7px] indeterminate:after:left-[3px] indeterminate:after:h-0 indeterminate:after:w-[9px] indeterminate:after:scale-100 indeterminate:after:rotate-0 indeterminate:after:border-r-0 indeterminate:after:opacity-100 forced-colors:after:border-[HighlightText] forced-colors:indeterminate:border-[Highlight] forced-colors:indeterminate:bg-[Highlight]`,
	radio: `${checkBase} rounded-full after:inset-[3.5px] after:scale-40 after:rounded-full after:bg-accent-text forced-colors:after:bg-[HighlightText]`
} as const;
