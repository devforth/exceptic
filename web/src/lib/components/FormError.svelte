<!--
@component
What a FormState could not place at a field: a refusal that names none, or a failure; announced
as it appears.
-->
<script lang="ts">
	import type { FormState } from '#lib/form.svelte.ts';
	import Notice from './Notice.svelte';

	let { form }: { form: FormState } = $props();
</script>

{#if form.error}<Notice tone="error" alert>{form.error}</Notice>{/if}
