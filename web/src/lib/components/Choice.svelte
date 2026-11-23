<!--
@component
One option of a ChoiceGroup: a radio button or checkbox with its label (text, or a snippet such
as a badge) and a muted description or a Hint beside it; a disabled one says why (reason).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { checkClasses } from './field.ts';
	import Hint from './Hint.svelte';

	type Props = {
		type: 'radio' | 'checkbox';
		checked: boolean;
		onchange: (checked: boolean) => void;
		/** Form fields, for a form that is sent as it is. */
		name?: string;
		value?: string;
		label?: string;
		description?: string;
		/** A "?" after the label, outside it, so a click on it does not toggle the option; it
		 * describes the option. */
		hint?: string;
		disabled?: boolean;
		/** Why it is disabled, e.g. "set SMTP_HOST". */
		reason?: string;
		children?: Snippet;
	};

	let {
		type,
		checked,
		onchange,
		name,
		value,
		label,
		description,
		hint,
		disabled = false,
		reason,
		children
	}: Props = $props();

	const id = $props.id();
</script>

{#snippet option()}
	<label
		class={[
			// With a description under it, the control sits by the first line (the name), not the middle.
			'flex gap-2 py-0.75',
			description ? 'items-start' : 'items-center',
			disabled ? 'cursor-not-allowed text-muted' : 'cursor-pointer'
		]}
	>
		<input
			{type}
			{name}
			{value}
			{checked}
			{disabled}
			aria-describedby={hint ? `${id}-hint` : undefined}
			class={[checkClasses[type], description && 'mt-[1.5px]']}
			onchange={(event) => onchange(event.currentTarget.checked)}
		/>
		<span
			class={[
				'flex min-w-0 gap-x-1',
				description ? 'flex-col items-start' : 'flex-wrap items-baseline'
			]}
		>
			{#if children}{@render children()}{:else}{label}{/if}
			{#if description}<span class="text-xs text-muted">{description}</span>{/if}
			{#if disabled && reason}<span class="text-xs text-muted">({reason})</span>{/if}
		</span>
	</label>
{/snippet}

{#if hint}
	<span class="flex items-center gap-1.25">
		{@render option()}
		<Hint text={hint} id="{id}-hint" />
	</span>
{:else}
	{@render option()}
{/if}
