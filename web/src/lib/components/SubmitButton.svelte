<!--
@component
The submit button of a form sent through a FormState: busy while it sends, and while the values
of an edit form are the saved ones it does nothing and says so in its tooltip (aria-disabled, so
it keeps the focus).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { FormState } from '#lib/form.svelte.ts';
	import Button from './Button.svelte';

	type Props = {
		form: FormState;
		/** The id of the form it sends, when it sits outside it (a dialog's footer). */
		formId?: string;
		size?: 'default' | 'large';
		/** Save by default. */
		children?: Snippet;
	};

	let { form, formId, size, children }: Props = $props();
</script>

<Button
	variant="primary"
	type="submit"
	form={formId}
	{size}
	busy={form.busy}
	unavailable={form.changed ? undefined : 'Nothing changed'}
>
	{#if children}{@render children()}{:else}Save{/if}
</Button>
