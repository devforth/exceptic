<!--
@component
A modal window with a title and a close button; Esc and a click beside it close it too.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Button from './Button.svelte';

	type Props = { title: string; open: boolean; children: Snippet };

	let { title, open = $bindable(), children }: Props = $props();

	let dialog: HTMLDialogElement;

	$effect(() => {
		if (open && !dialog.open) {
			dialog.showModal();
		} else if (!open && dialog.open) {
			dialog.close();
		}
	});

	// A click beside the content lands on the dialog element itself (the backdrop).
	function closeOutside(event: MouseEvent) {
		if (event.target === dialog) {
			open = false;
		}
	}
</script>

<dialog
	bind:this={dialog}
	onclose={() => (open = false)}
	onclick={closeOutside}
	aria-label={title}
	class="backdrop:bg-black/50 m-auto max-h-[90vh] w-[min(1000px,calc(100vw-32px))] rounded-dialog bg-surface p-0 text-text shadow-pop"
>
	<div class="flex items-center justify-between gap-3 border-b border-border px-4 py-2">
		<strong>{title}</strong>
		<Button variant="ghost" icon="close" title="Close" onclick={() => (open = false)} />
	</div>
	<div class="max-h-[calc(90vh-56px)] overflow-auto p-4">{@render children()}</div>
</dialog>
