// The variants of Button.svelte (a .ts module, so pages and eslint resolve the type).
export type ButtonVariant =
	| 'default'
	| 'primary'
	| 'danger'
	| 'info'
	| 'neutral'
	| 'accent'
	| 'menu'
	| 'link'
	| 'muted'
	| 'title'
	| 'ghost';

/** The look of a copy button: blue, or grey for the MCP page's commands. */
export type CopyVariant = Extract<ButtonVariant, 'info' | 'neutral'>;
