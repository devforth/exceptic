<!--
@component
A small form in a Dialog (rename, change a color): its fields, Cancel and the submit button.
The request goes through a FormState, so a refused field shows its problem beside it and the
dialog stays open; it closes once the request succeeds. Save does nothing while nothing changed
(the caller resets the form when it opens it for a record). Enter in a field submits it; the first
text field is selected when it opens.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { FormState } from '#lib/form.svelte.ts';
	import Button from './Button.svelte';
	import Dialog from './Dialog.svelte';
	import { formClasses } from './field.ts';
	import FormError from './FormError.svelte';
	import SubmitButton from './SubmitButton.svelte';

	type Props = {
		title: string;
		open: boolean;
		/** Its fields read form.fields for their problems. */
		form: FormState;
		/** Sends the form; the dialog closes when it resolves. */
		save: () => Promise<unknown>;
		/** The submit button's label: Save by default. */
		submitLabel?: string;
		children: Snippet;
	};

	let { title, open = $bindable(), form, save, submitLabel, children }: Props = $props();

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
	<form bind:this={element} id="{id}-form" class={formClasses} onsubmit={submit}>
		{@render children()}
		<FormError {form} />
	</form>
	{#snippet actions()}
		<Button disabled={form.busy} onclick={() => (open = false)}>Cancel</Button>
		<SubmitButton {form} formId="{id}-form">{submitLabel ?? 'Save'}</SubmitButton>
	{/snippet}
</Dialog>
