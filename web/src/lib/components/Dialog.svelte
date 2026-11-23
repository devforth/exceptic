<!--
@component
A modal window; Esc and a click beside it close it, unless it is not dismissible (while its
action runs). `wide` (a log, a summary) has a bar with its title and Close; `narrow` fits a
question or a short form: its title, the content and its buttons (`actions`, which hold its
Cancel) in one padded column. onopen runs once it is shown (to move the focus).
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

	const titleId = $props.id();

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
	aria-labelledby={titleId}
	class={[
		// whitespace-normal: a dialog opened from a nowrap place (a table's actions cell) wraps its text.
		'm-auto max-h-[90vh] border border-border bg-surface p-0 whitespace-normal text-text shadow-pop backdrop:bg-backdrop',
		size === 'wide'
			? 'w-[min(1000px,calc(100vw-32px))] rounded-popup'
			: 'w-[min(420px,calc(100vw-32px))] rounded-dialog'
	]}
>
	{#if size === 'wide'}
		<div class="flex items-center justify-between gap-3 border-b border-border px-3.5 py-2.5">
			<h2 id={titleId} class="font-bold">{title}</h2>
			<Button onclick={dismiss}>Close</Button>
		</div>
		<div
			class={[
				'overflow-auto p-3.5',
				actions ? 'max-h-[calc(90vh-112px)]' : 'max-h-[calc(90vh-58px)]'
			]}
		>
			{@render children()}
		</div>
		{#if actions}
			<div class="flex justify-end gap-2 px-3.5 pb-3.5">{@render actions()}</div>
		{/if}
	{:else}
		<div class="flex max-h-[calc(90vh-2px)] flex-col gap-3.5 p-5">
			<h2 id={titleId} class="text-dialog font-bold">{title}</h2>
			<!-- Room for the focus rings of fields at its edges. -->
			<div class="-m-1 min-h-0 overflow-auto p-1">{@render children()}</div>
			{#if actions}
				<div class="flex justify-end gap-2">{@render actions()}</div>
			{/if}
		</div>
	{/if}
</dialog>
