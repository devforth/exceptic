<!--
@component
The end of a form sent through a FormState: its error (FormError), then the submit button and,
with `cancel`, a Cancel link back to that page, in one row. `stretch` puts the submit button
across the form instead (a login, the one field of a page).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { FormState } from '#lib/form.svelte.ts';
	import Button from './Button.svelte';
	import FormError from './FormError.svelte';
	import SubmitButton from './SubmitButton.svelte';

	type Props = {
		form: FormState;
		/** Where Cancel goes; no Cancel without it. */
		cancel?: string;
		size?: 'default' | 'large';
		stretch?: boolean;
		/** The submit button's text; Save by default. */
		children?: Snippet;
	};

	let { form, cancel, size, stretch = false, children }: Props = $props();
</script>

<FormError {form} />
<div class={stretch ? 'flex flex-col' : 'flex flex-wrap gap-2'}>
	<SubmitButton {form} {size}>
		{#if children}{@render children()}{:else}Save{/if}
	</SubmitButton>
	{#if cancel}<Button href={cancel}>Cancel</Button>{/if}
</div>
