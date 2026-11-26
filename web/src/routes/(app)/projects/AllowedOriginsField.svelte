<!--
@component
The sites whose browsers may send to a project, typed as a list; empty allows any site. Its
container (a Details or FormGroup) is titled Allowed domains.
-->
<script lang="ts">
	import { help } from '#lib/api/catalog.gen.ts';
	import TextField from '#lib/components/TextField.svelte';

	type Props = { value: string; error?: string };

	let { value = $bindable(), error }: Props = $props();
</script>

<TextField
	label="Domains"
	name="allowed_origins"
	type="text"
	bind:value
	placeholder="https://app.example.com, https://*.example.com"
	hint={help.fields.Projects['Allowed domains']}
	description="Separated by commas. Empty: any site. Backend SDKs are not checked."
	{error}
/>
