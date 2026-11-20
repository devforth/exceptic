<!--
@component
A value to copy (a DSN, a command): read-only, selected on focus, with its copy button.
`masked` hides a secret behind dots until Show; the button copies it either way.
-->
<script lang="ts">
	import Button from './Button.svelte';
	import CopyButton from './CopyButton.svelte';
	import { fieldClasses } from './field.ts';

	type Props = {
		value: string;
		/** The field's accessible name, e.g. "DSN of backend". */
		label: string;
		/** What the button does, e.g. "Copy the DSN of backend". */
		copyTitle: string;
		copied?: string;
		masked?: boolean;
	};

	let { value, label, copyTitle, copied, masked = false }: Props = $props();

	let shown = $state(false);

	const hidden = $derived(masked && !shown);
</script>

<div class="flex min-w-0 items-center gap-1.5">
	<input
		class={[fieldClasses, 'min-w-0 flex-1 bg-code-bg font-mono text-code']}
		value={hidden ? '•'.repeat(Math.min(value.length, 32)) : value}
		readonly
		aria-label={label}
		onfocus={(event) => event.currentTarget.select()}
	/>
	{#if masked}
		<Button
			icon={shown ? 'hide' : 'view'}
			title={shown ? 'Hide' : 'Show'}
			expanded={shown}
			onclick={() => (shown = !shown)}
		/>
	{/if}
	<CopyButton text={value} title={copyTitle} {copied} />
</div>
