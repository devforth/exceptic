<!--
@component
A secret shown only this once (a temporary password, a personal token), in a box in the warning
tone: what to do with it, then the value to copy, plain or masked until Show. The copy button
copies the full value.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import CopyField from './CopyField.svelte';

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

<div role="status" class="flex flex-col gap-2 rounded-note border border-warning bg-warning-bg p-3">
	<div>{@render children()}</div>
	<div class="max-w-130">
		<CopyField {value} {label} {copyTitle} {copied} {masked} />
	</div>
</div>
