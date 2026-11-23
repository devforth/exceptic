<!--
@component
One option of a ChoiceGroup: a radio button or checkbox with its label (text, or a snippet such
as a badge) and a muted description or a Hint beside it; a disabled one says why (reason).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
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
			'flex items-start gap-2',
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
			class="mt-[3px] size-4 shrink-0 accent-accent focus-ring"
			onchange={(event) => onchange(event.currentTarget.checked)}
		/>
		<span class="flex min-w-0 flex-wrap items-baseline gap-x-2">
			{#if children}{@render children()}{:else}{label}{/if}
			{#if description}<span class="text-xs text-muted">{description}</span>{/if}
			{#if disabled && reason}<span class="text-xs text-muted">({reason})</span>{/if}
		</span>
	</label>
{/snippet}

{#if hint}
	<span class="flex items-start gap-1.5">
		{@render option()}
		<span class="mt-[3px] flex"><Hint text={hint} id="{id}-hint" /></span>
	</span>
{:else}
	{@render option()}
{/if}
