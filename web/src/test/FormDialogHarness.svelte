<!-- A FormDialog with one field, open, for its test. -->
<script lang="ts">
	import FormDialog from '#lib/components/FormDialog.svelte';
	import TextField from '#lib/components/TextField.svelte';
	import { FormState } from '#lib/form.svelte.ts';

	let { save }: { save: () => Promise<unknown> } = $props();

	let open = $state(true);
	let name = $state('backend');
	const form = new FormState(() => ({ name: name.trim() }));
</script>

<FormDialog title="Rename project" bind:open {form} {save}>
	<TextField label="Name" name="name" bind:value={name} error={form.fields.name} />
</FormDialog>
