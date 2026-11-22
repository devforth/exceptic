<!--
@component
A small form in a Dialog (rename, change a color): its fields, Cancel and the submit button.
The request goes through a FormState, so a refused field shows its problem beside it and the
dialog stays open; it closes once the request succeeds. Enter in a field submits it; the first
text field is selected when it opens.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { FormState } from '#lib/form.svelte.ts';
	import Button from './Button.svelte';
	import Dialog from './Dialog.svelte';
	import Notice from './Notice.svelte';

	type Props = {
		title: string;
		open: boolean;
		/** Its fields read form.fields for their problems. */
		form: FormState;
		/** Sends the form; the dialog closes when it resolves. */
		save: () => Promise<unknown>;
		submitLabel?: string;
		children: Snippet;
	};

	let { title, open = $bindable(), form, save, submitLabel = 'Save', children }: Props = $props();

	const id = $props.id();
	let element: HTMLFormElement | undefined = $state();

	function selectFirstField() {
		element?.querySelector<HTMLInputElement>('input[type="text"], input:not([type])')?.select();
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();

		if (await form.submit(save)) open = false;
	}
</script>

<Dialog {title} bind:open size="narrow" dismissible={!form.busy} onopen={selectFirstField}>
	<form bind:this={element} id="{id}-form" class="flex flex-col gap-3" onsubmit={submit}>
		{@render children()}
		{#if form.error}<Notice tone="error">{form.error}</Notice>{/if}
	</form>
	{#snippet actions()}
		<Button disabled={form.busy} onclick={() => (open = false)}>Cancel</Button>
		<Button variant="primary" type="submit" form="{id}-form" busy={form.busy}>{submitLabel}</Button>
	{/snippet}
</Dialog>
