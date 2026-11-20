<!--
@component
A modal window with a title and a close button; Esc and a click beside it close it too, unless
it is not dismissible (while its action runs). `narrow` fits a question or a short form, with
its buttons in `actions` at the bottom; onopen runs once it is shown (to move the focus).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Button from './Button.svelte';

	type Props = {
		title: string;
		open: boolean;
		size?: 'wide' | 'narrow';
		dismissible?: boolean;
		onopen?: () => void;
		actions?: Snippet;
		children: Snippet;
	};

	let {
		title,
		open = $bindable(),
		size = 'wide',
		dismissible = true,
		onopen,
		actions,
		children
	}: Props = $props();

	let dialog: HTMLDialogElement;

	$effect(() => {
		if (open && !dialog.open) {
			dialog.showModal();
			onopen?.();
		} else if (!open && dialog.open) {
			dialog.close();
		}
	});

	function dismiss() {
		if (dismissible) open = false;
	}

	// A click beside the content lands on the dialog element itself (the backdrop).
	function closeOutside(event: MouseEvent) {
		if (event.target === dialog) dismiss();
	}

	// The browser may close a modal on its own (a close watcher); one that must stay opens again.
	function closed() {
		if (open && !dismissible) {
			dialog.showModal();

			return;
		}

		open = false;
	}

	function cancel(event: Event) {
		if (!dismissible) event.preventDefault();
	}
</script>

<dialog
	bind:this={dialog}
	onclose={closed}
	oncancel={cancel}
	onclick={closeOutside}
	aria-label={title}
	class={[
		'backdrop:bg-black/50 m-auto max-h-[90vh] rounded-dialog bg-surface p-0 text-text shadow-pop',
		size === 'wide' ? 'w-[min(1000px,calc(100vw-32px))]' : 'w-[min(440px,calc(100vw-32px))]'
	]}
>
	<div class="flex items-center justify-between gap-3 border-b border-border px-4 py-2">
		<strong>{title}</strong>
		<Button variant="ghost" icon="close" title="Close" onclick={dismiss} />
	</div>
	<div
		class={['overflow-auto p-4', actions ? 'max-h-[calc(90vh-112px)]' : 'max-h-[calc(90vh-56px)]']}
	>
		{@render children()}
	</div>
	{#if actions}
		<div class="flex justify-end gap-2 px-4 pb-4">{@render actions()}</div>
	{/if}
</dialog>
