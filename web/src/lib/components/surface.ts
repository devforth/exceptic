// The surface of whatever pops up over the page (a Picker's options, a RowMenu): one source. A
// long list scrolls inside it, at most 320px or 60% of the window tall.
export const popupClasses =
	'max-h-[min(320px,60vh)] overflow-y-auto rounded-popup border border-border bg-surface p-1 shadow-pop';

// The raised surface of a card (Card, a Details section, a ChoiceGroup card): one source.
export const cardClasses = 'rounded-card bg-surface shadow-card forced-colors:border';

/** A box inside a card: a scroll box, a nested section, an exception. */
export const boxClasses = 'rounded-box border border-border bg-surface';
export const cardPadding = 'px-[22px] py-5';
