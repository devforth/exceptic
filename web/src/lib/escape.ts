// Esc on a page goes back one level (Breadcrumbs), unless it belongs to something else: a field
// being typed in, or an open dialog, menu or dropdown, which closes instead. A popup that is not
// a dialog carries data-popup (a <details> while it is open, any other element while it exists);
// one that closes itself on Esc also prevents the event's default.
const openPopups = 'dialog[open], details[open][data-popup], [data-popup]:not(details)';
const fields = 'input, textarea, select, [contenteditable]:not([contenteditable="false"])';

/** Whether this keydown is an Esc that should go back. */
export function escapeGoesBack(event: KeyboardEvent): boolean {
	if (event.key !== 'Escape' || event.defaultPrevented) return false;

	if (event.target instanceof Element && event.target.closest(fields)) return false;

	return document.querySelector(openPopups) === null;
}
