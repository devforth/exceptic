// Copies text. Without a secure context (plain HTTP on a LAN address) navigator.clipboard
// is missing, so a selected off-screen textarea is copied with execCommand instead.
export async function copyText(text: string): Promise<boolean> {
	try {
		await navigator.clipboard.writeText(text);

		return true;
	} catch {
		return copyWithSelection(text);
	}
}

function copyWithSelection(text: string): boolean {
	const previousFocus = document.activeElement;
	const field = document.createElement('textarea');
	field.value = text;
	field.readOnly = true;
	field.tabIndex = -1;
	field.setAttribute('aria-hidden', 'true');
	field.className = 'pointer-events-none fixed top-0 -left-[9999px] size-px opacity-0';
	document.body.append(field);

	try {
		field.focus({ preventScroll: true });
		field.select();

		// Deprecated, but the only way to copy without a secure context.
		return document.execCommand('copy');
	} catch {
		return false;
	} finally {
		field.remove();

		if (previousFocus instanceof HTMLElement) {
			previousFocus.focus({ preventScroll: true });
		}
	}
}
