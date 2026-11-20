// The question of ConfirmDialog (a .ts module, so pages and eslint resolve the type).
export type Confirmation = {
	title: string;
	message: string;
	/** The button that acts, e.g. "Delete". */
	confirmLabel: string;
	/** danger (the default) for what cannot be undone. */
	tone?: 'danger' | 'primary';
};

/** An action run after a confirmation; a failure is shown in the dialog. */
export type ConfirmedAction = () => unknown;
