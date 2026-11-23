<!--
@component
Copies text and says so in the toast. Without a label it is an icon; title names it.
-->
<script lang="ts">
	import { copyText } from '#lib/clipboard.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import Button from './Button.svelte';

	type Props = {
		text: string;
		/** What the button does, e.g. "Copy DSN": its tooltip and accessible name. */
		title: string;
		label?: string;
		/** The toast after a copy. */
		copied?: string;
	};

	let { text, title, label, copied = 'Copied to clipboard.' }: Props = $props();

	let busy = false;
	let done = $state(false);
	let doneTimer: ReturnType<typeof setTimeout> | undefined;

	// Never disabled: a focused button that turns disabled drops keyboard focus.
	async function copy() {
		if (busy) return;

		busy = true;

		const ok = await copyText(text).finally(() => (busy = false));

		if (ok) toast.show(copied, 'info');
		else toast.show('Could not copy. Select the text and copy it with Ctrl+C or ⌘C.', 'error');

		if (ok) {
			done = true;
			clearTimeout(doneTimer);
			doneTimer = setTimeout(() => (done = false), 2000);
		}
	}
</script>

{#snippet labelText()}{label}{/snippet}

<Button
	variant="info"
	icon={done ? 'check' : 'copy'}
	title={done ? 'Copied!' : title}
	onclick={copy}
	children={label ? labelText : undefined}
/>
