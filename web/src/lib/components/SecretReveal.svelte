<!--
@component
A secret shown only this once (a temporary password, a personal token): what to do with it in
a warning, the value to copy, plain or masked until Show. The copy button copies the full value.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import CopyField from './CopyField.svelte';
	import Notice from './Notice.svelte';

	type Props = {
		value: string;
		/** Its name, e.g. "Temporary password". */
		label: string;
		copyTitle: string;
		copied?: string;
		masked?: boolean;
		/** What to do with it, e.g. "Give it to the user. It is shown only now." */
		children: Snippet;
	};

	let { value, label, copyTitle, copied, masked = false, children }: Props = $props();
</script>

<div role="status" class="flex flex-col gap-2">
	<Notice tone="warning">{@render children()}</Notice>
	<CopyField {value} {label} {copyTitle} {copied} {masked} />
</div>
