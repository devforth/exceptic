<!--
@component
Asks before an action: the question, Cancel (focused first) and the button that acts. Cancel,
Esc and a click beside it close it without acting. While the action runs the dialog stays and
cannot be dismissed; it closes when the action is done, or shows why it failed.
-->
<script lang="ts">
	import { errorMessage } from '#lib/api/client.ts';
	import Button from './Button.svelte';
	import type { ConfirmedAction, Confirmation } from './confirm.ts';
	import Dialog from './Dialog.svelte';
	import Notice from './Notice.svelte';

	type Props = Confirmation & { open: boolean; onconfirm: ConfirmedAction };

	let {
		open = $bindable(),
		title,
		message,
		confirmLabel,
		tone = 'danger',
		onconfirm
	}: Props = $props();

	let busy = $state(false);
	let failure = $state('');
	let footer: HTMLElement | undefined = $state();

	function opened() {
		failure = '';
		footer?.querySelector('button')?.focus();
	}

	async function confirm() {
		busy = true;
		failure = '';

		try {
			await onconfirm();
			open = false;
		} catch (error) {
			failure = errorMessage(error, 'Something went wrong. Try again.');
		} finally {
			busy = false;
		}
	}
</script>

<Dialog {title} bind:open size="narrow" dismissible={!busy} onopen={opened}>
	<p>{message}</p>
	{#if failure}
		<div class="mt-3"><Notice tone="error">{failure}</Notice></div>
	{/if}
	{#snippet actions()}
		<div bind:this={footer} class="contents">
			<Button disabled={busy} onclick={() => (open = false)}>Cancel</Button>
			<Button variant={tone} {busy} onclick={confirm}>{confirmLabel}</Button>
		</div>
	{/snippet}
</Dialog>
