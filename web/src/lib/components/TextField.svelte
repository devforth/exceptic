<!--
@component
A labelled text field of a form, with its Hint, a note after the label ("optional"), a description
under the field and the error the server gave it. The limits (maxlength, pattern, …) come from the
generated ones of the server, so the browser checks what the server checks. `saved`: a stored
secret, its mask in the placeholder and in a description for screen readers (a placeholder is not
reliably read, and goes once typing starts); the field may stay empty to keep it. `action`: a
button beside the field (Verify, Find chat ID). `variant="hero"`: the one large, centered field
of a page, its label only for screen readers; `variant="code"`: a short code (a confirmation
code), narrow and monospace.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { fieldClasses, heroFieldClasses } from './field.ts';
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
		/** After the label, muted: "optional". */
		note?: string;
		/** Under the field, muted: what happens with the value. */
		description?: string;
		error?: string;
		/** The mask of a stored secret. */
		saved?: string;
		action?: Snippet;
		variant?: 'default' | 'hero' | 'code';
		/** Takes the focus when shown. */
		autofocus?: boolean;
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
		description,
		error,
		saved,
		action,
		variant = 'default',
		autofocus = false
	}: Props = $props();

	const hero = $derived(variant === 'hero');

	function focus(input: HTMLInputElement) {
		if (autofocus) input.focus();
	}

	const id = $props.id();

	const described = $derived(
		[description && `${id}-description`, saved && `${id}-saved`, error && `${id}-error`]
			.filter(Boolean)
			.join(' ') || undefined
	);
</script>

{#snippet input()}
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
		placeholder={saved ? `saved: ${saved} — leave empty to keep` : placeholder}
		{autocomplete}
		aria-invalid={error ? 'true' : undefined}
		aria-describedby={described}
		{@attach focus}
		class={[
			hero
				? heroFieldClasses
				: [
						fieldClasses,
						variant === 'code'
							? 'w-[140px] font-mono text-sm tracking-[.15em]'
							: 'w-full min-w-0 text-sm'
					],
			'bg-surface'
		]}
	/>
{/snippet}

<div class="flex flex-col gap-1">
	<span class={hero ? 'sr-only' : 'flex items-center gap-1.25 text-label text-muted'}>
		<label for={id}>{label}</label>
		{#if note}<span class="text-xs">({note})</span>{/if}
		{#if hint}<Hint text={hint} />{/if}
	</span>
	{#if action}
		<span class="flex gap-2">
			{@render input()}
			{@render action()}
		</span>
	{:else}
		{@render input()}
	{/if}
	{#if description}
		<p id="{id}-description" class="text-xs text-muted">{description}</p>
	{/if}
	{#if saved}
		<span id="{id}-saved" class="sr-only">Saved: {saved}. Leave it empty to keep it.</span>
	{/if}
	{#if error}
		<p id="{id}-error" class="text-xs text-error">{error}</p>
	{/if}
</div>
