<!--
@component
A page title with a pencil that turns it into a field of the form around it: the form's own Save
sends it (Enter too), Esc goes back to the title and drops what was typed. A refused value
(`error`) opens the field with its problem. The rules (maxlength, pattern) are the server's.
-->
<script lang="ts">
	import { tick } from 'svelte';
	import Button from './Button.svelte';
	import { fieldFrame } from './field.ts';
	import { titleClasses, type TitleField } from './heading.ts';

	type Props = TitleField & { title: string; value: string };

	let {
		title,
		value = $bindable(),
		label,
		name = 'name',
		maxlength,
		pattern,
		placeholder,
		error
	}: Props = $props();

	const id = $props.id();

	let editing = $state(false);
	let pencil: HTMLElement | undefined = $state();

	const open = $derived(editing || Boolean(error));

	// A saved name comes back as the new title: the field closes.
	// svelte-ignore state_referenced_locally
	let shownTitle = title;

	$effect.pre(() => {
		if (title !== shownTitle) {
			shownTitle = title;
			editing = false;
		}
	});

	function selectAll(input: HTMLInputElement) {
		input.select();
	}

	async function keydown(event: KeyboardEvent) {
		if (event.key !== 'Escape') return;

		// The field's Esc cancels; it does not leave the page.
		event.preventDefault();
		event.stopPropagation();
		value = title;
		editing = false;
		await tick();
		pencil?.querySelector('button')?.focus();
	}
</script>

{#if open}
	<div class="flex min-w-0 flex-col gap-1">
		<input
			{name}
			bind:value
			aria-label={label}
			required
			{maxlength}
			{pattern}
			{placeholder}
			autocomplete="off"
			aria-invalid={error ? 'true' : undefined}
			aria-describedby={error ? `${id}-error` : undefined}
			onkeydown={keydown}
			{@attach selectAll}
			class={[fieldFrame, 'h-10 w-80 max-w-full bg-surface', titleClasses]}
		/>
		{#if error}<p id="{id}-error" class="text-xs text-error">{error}</p>{/if}
	</div>
{:else}
	<div class="flex min-w-0 items-center gap-1">
		<h1 class={titleClasses}>{title}</h1>
		<span bind:this={pencil}>
			<Button variant="ghost" icon="edit" title="Rename" onclick={() => (editing = true)} />
		</span>
	</div>
{/if}
