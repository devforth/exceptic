<!--
@component
A labelled text field of a form, with its Hint, a note after the label ("optional: …") and the
error the server gave it. The limits (maxlength, pattern, …) come from the generated ones of the
server, so the browser checks what the server checks. `saved`: a stored secret, shown masked; the
field may stay empty to keep it.
-->
<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { fieldClasses } from './field.ts';
	import Hint from './Hint.svelte';

	type Props = {
		label: string;
		name: string;
		value?: string;
		type?: 'text' | 'email' | 'password' | 'url';
		readonly?: boolean;
		required?: boolean;
		maxlength?: number;
		minlength?: number;
		pattern?: string;
		placeholder?: string;
		autocomplete?: HTMLInputAttributes['autocomplete'];
		hint?: string;
		/** After the label, muted: "optional: the username when empty". */
		note?: string;
		error?: string;
		/** The mask of a stored secret. */
		saved?: string;
	};

	let {
		label,
		name,
		value = $bindable(''),
		type = 'text',
		readonly = false,
		required = false,
		maxlength,
		minlength,
		pattern,
		placeholder,
		autocomplete,
		hint,
		note,
		error,
		saved
	}: Props = $props();

	const id = $props.id();

	const described = $derived(
		[saved && `${id}-saved`, error && `${id}-error`].filter(Boolean).join(' ') || undefined
	);
</script>

<div class="flex flex-col gap-1">
	<span class="flex items-center gap-1.5 text-label text-muted">
		<label for={id}>{label}</label>
		{#if note}<span class="text-xs">({note})</span>{/if}
		{#if hint}<Hint text={hint} />{/if}
	</span>
	<input
		{id}
		{name}
		{type}
		bind:value
		{readonly}
		required={required && !saved}
		{maxlength}
		{minlength}
		{pattern}
		{placeholder}
		{autocomplete}
		aria-invalid={error ? 'true' : undefined}
		aria-describedby={described}
		class={[
			fieldClasses,
			'w-full text-sm',
			readonly ? 'bg-code-bg' : 'bg-surface',
			error && 'border-error focus-visible:border-error'
		]}
	/>
	{#if saved}
		<p id="{id}-saved" class="text-xs text-muted">
			Saved: <code>{saved}</code>. Leave it empty to keep it.
		</p>
	{/if}
	{#if error}
		<p id="{id}-error" class="text-xs text-error">{error}</p>
	{/if}
</div>
