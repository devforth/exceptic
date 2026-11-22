// The items of RowMenu.svelte (a .ts module, so pages and eslint resolve the type).
import type { IconName } from '#lib/icons.ts';
import type { ConfirmedAction, Confirmation } from './confirm.ts';

export type MenuItem = {
	label: string;
	icon: IconName;
	/** Red: what deletes or cannot be undone. */
	danger?: boolean;
	/** A line above it: destructive items come last, after one. */
	separatorBefore?: boolean;
} & (
	| { href: string; action?: never; confirm?: never; disabled?: never }
	| {
			action: ConfirmedAction;
			href?: never;
			/** Asked in a ConfirmDialog before the action runs. */
			confirm?: Confirmation;
			disabled?: never;
	  }
	| {
			/** Why it cannot be chosen: shown under the label. */
			disabled: string;
			href?: never;
			action?: never;
			confirm?: never;
	  }
);
