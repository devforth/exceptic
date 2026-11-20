<!--
@component
A value to copy (a DSN, a command): read-only, selected on focus, with its copy button.
-->
<script lang="ts">
	import CopyButton from './CopyButton.svelte';
	import { fieldClasses } from './field.ts';

	type Props = {
		value: string;
		/** The field's accessible name, e.g. "DSN of backend". */
		label: string;
		/** What the button does, e.g. "Copy the DSN of backend". */
		copyTitle: string;
		copied?: string;
	};

	let { value, label, copyTitle, copied }: Props = $props();
</script>

<div class="flex min-w-0 items-center gap-1.5">
	<input
		class={[fieldClasses, 'min-w-0 flex-1 bg-code-bg font-mono text-code']}
		{value}
		readonly
		aria-label={label}
		onfocus={(event) => event.currentTarget.select()}
	/>
	<CopyButton text={value} title={copyTitle} {copied} />
</div>
