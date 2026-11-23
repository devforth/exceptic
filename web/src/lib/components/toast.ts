// The corners of Toast.svelte (a .ts module, so the layout and eslint resolve the type).
export type ToastPosition =
	'top-left' | 'top-center' | 'top-right' | 'bottom-left' | 'bottom-center' | 'bottom-right';

// Literal class names, so Tailwind builds them.
export const toastPositions: Record<ToastPosition, string> = {
	'top-left': 'top-5 left-5',
	'top-center': 'top-5 left-1/2 -translate-x-1/2',
	'top-right': 'top-5 right-5',
	'bottom-left': 'bottom-5 left-5',
	'bottom-center': 'bottom-5 left-1/2 -translate-x-1/2',
	'bottom-right': 'right-5 bottom-5'
};
