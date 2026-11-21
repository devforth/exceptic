<!--
@component
The name of a project, with the server's rules for it (the generated limits).
-->
<script lang="ts">
	import { limits } from '#lib/api/catalog.gen.ts';
	import TextField from '#lib/components/TextField.svelte';

	let { value = $bindable(), error }: { value: string; error?: string } = $props();
</script>

<TextField
	label="Name"
	name="name"
	bind:value
	required
	maxlength={limits.projectName}
	pattern={limits.projectNamePattern}
	placeholder="backend"
	autocomplete="off"
	{error}
/>
