<!--
@component
A value to copy (a DSN, a command): read-only, selected on focus, with its copy button.
-->
<script lang="ts">
	import CopyButton from './CopyButton.svelte';

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
		class="h-9 min-w-0 flex-1 rounded-control border border-border bg-code-bg px-2 font-mono text-code text-text focus-visible:border-accent focus-visible:outline-none"
		{value}
		readonly
		aria-label={label}
		onfocus={(event) => event.currentTarget.select()}
	/>
	<CopyButton text={value} title={copyTitle} {copied} />
</div>
