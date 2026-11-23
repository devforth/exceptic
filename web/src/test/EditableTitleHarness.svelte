<!-- A form around an EditableTitle, for its tests: Submit reports the bound value. -->
<script lang="ts">
	import EditableTitle from '#lib/components/EditableTitle.svelte';

	type Props = { title: string; error?: string; onsubmit: (name: string) => void };

	let { title, error, onsubmit }: Props = $props();

	// svelte-ignore state_referenced_locally
	let value = $state(title);
</script>

<form
	onsubmit={(event) => {
		event.preventDefault();
		onsubmit(value);
	}}
>
	<EditableTitle {title} label="Project name" bind:value {error} />
	<button type="submit">Submit</button>
</form>
