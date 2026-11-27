// What pops up from a control (a Select's list, a DateRange's calendar): the control's look
// while it is open, a place fixed under it, so a table's scroll box does not cut it (above it
// when there is no room below, inside the viewport), and what closes it.

/** The control whose popup is open: outlined as when focused. */
export const openControlClasses = 'border-accent shadow-[0_0_0_3px_var(--color-focus)]';

/** The chevron of such a control, turned up while its popup is open. */
export function chevronClasses(open: boolean) {
	return [
		'flex text-muted transition-transform motion-reduce:transition-none',
		open && 'rotate-180'
	];
}

/** The least width of a dropdown list (a Picker's, a Select's), in px: wider when its control or
 * its values need it. */
export const listMinWidth = 224;

/** The same as a class beside a control as wide as it (Picker): 224px = 14rem. */
export const listMinWidthClass = 'min-w-[max(100%,14rem)]';

const gap = 4;
const margin = 8;

export type PopupPlace = { top: number; left: number; minWidth: number };

/** Where a popup of this size goes for a control at this box: under it, or above it when there
 * is no room below, its left edge moved in so the wider of the two stays inside the viewport. */
export function popupPlace(control: DOMRect, popup: { width: number; height: number }): PopupPlace {
	const { height } = popup;
	// The viewport without the scrollbar: position: fixed measures from its edge.
	const width = document.documentElement.clientWidth;
	const below = control.bottom + gap;
	const above = control.top - height - gap;
	const top = below + height > innerHeight - margin && above > margin ? above : below;

	return {
		top,
		left: Math.max(
			margin,
			Math.min(control.left, width - Math.max(control.width, popup.width) - margin)
		),
		minWidth: control.width
	};
}

/**
 * Calls close on a pointer down outside the given elements, on scrolling outside them and on
 * resizing; returns what removes the listeners (an $effect's cleanup). `refocus` is true when
 * the focus is inside them and nothing else moves it (scrolling, resizing): the popup then gives
 * it back to its control, so a keyboard user keeps their place.
 */
export function closeWhenLeft(
	inside: () => (Element | undefined)[],
	close: (refocus: boolean) => void
) {
	const within = (node: Node | null) => inside().some((element) => element?.contains(node));
	const pointer = (event: Event) => {
		if (!within(event.target as Node)) close(false);
	};
	const scroll = (event: Event) => {
		if (!within(event.target as Node)) close(within(document.activeElement));
	};
	const resize = () => close(within(document.activeElement));

	document.addEventListener('pointerdown', pointer);
	addEventListener('scroll', scroll, true);
	addEventListener('resize', resize);

	return () => {
		document.removeEventListener('pointerdown', pointer);
		removeEventListener('scroll', scroll, true);
		removeEventListener('resize', resize);
	};
}
