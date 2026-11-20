<!--
@component
A button whose action is asked first: it opens a ConfirmDialog and runs the action on confirm.
It looks like any Button of its variant (danger for a delete).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { IconName } from '#lib/icons.ts';
	import Button from './Button.svelte';
	import type { ButtonVariant } from './button.ts';
	import type { ConfirmedAction, Confirmation } from './confirm.ts';
	import ConfirmDialog from './ConfirmDialog.svelte';

	type Props = {
		confirm: Confirmation;
		action: ConfirmedAction;
		variant?: ButtonVariant;
		icon?: IconName;
		/** The name of an icon-only button, or a tooltip of one with text. */
		title?: string;
		children?: Snippet;
	};

	let { confirm, action, variant = 'danger', icon, title, children }: Props = $props();

	let open = $state(false);
</script>

<Button {variant} {icon} {title} popup="dialog" onclick={() => (open = true)} {children} />
<ConfirmDialog bind:open {...confirm} onconfirm={action} />
