<!--
@component
The name of a project, with the server's rules for it (the generated limits). `hero`: the one
large field of the new project page.
-->
<script lang="ts">
	import TextField from '#lib/components/TextField.svelte';
	import { projectNameRules } from '#lib/projects.ts';

	type Props = { value: string; error?: string; variant?: 'default' | 'hero' };

	let { value = $bindable(), error, variant = 'default' }: Props = $props();

	const hero = $derived(variant === 'hero');
</script>

<TextField
	label={hero ? 'Project name' : 'Name'}
	name="name"
	bind:value
	required
	{...projectNameRules}
	placeholder={hero ? 'Project name, e.g. backend' : 'backend'}
	autocomplete="off"
	{variant}
	autofocus={hero}
	{error}
/>
