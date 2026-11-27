<!--
@component
"+N more" after the values a row has room for: an outlined badge that opens the rest (children) in
a popup under it, fixed so a table's scroll box does not cut it. Esc, a click outside or a link
inside closes it. ProjectsSummary's `limit` uses it.
-->
<script lang="ts">
	import { tick, type Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import { closeWhenLeft, popupPlace, type PopupPlace } from './popup.ts';
	import { popupClasses } from './surface.ts';

	type Props = {
		/** How many values the popup holds. */
		count: number;
		/** The popup's accessible name and the button's tooltip, e.g. "19 more projects". */
		label: string;
		children: Snippet;
	};

	let { count, label, children }: Props = $props();

	let open = $state(false);
	let place: PopupPlace = $state({ top: 0, left: 0, minWidth: 0 });
	let box: HTMLElement | undefined = $state();
	let popup: HTMLElement | undefined = $state();

	const button = () => box?.querySelector('button');

	async function toggle() {
		if (open) return close(false);

		open = true;
		await tick();

		const control = button();

		if (control && popup) {
			place = popupPlace(control.getBoundingClientRect(), {
				width: popup.offsetWidth,
				height: popup.offsetHeight
			});
		}
	}

	function close(refocus: boolean) {
		open = false;

		if (refocus) button()?.focus();
	}

	function keydown(event: KeyboardEvent) {
		if (open && event.key === 'Escape') {
			// Handled: Esc must not also leave the page (Breadcrumbs).
			event.preventDefault();
			close(true);
		}
	}

	$effect(() => {
		if (!open) return;

		return closeWhenLeft(() => [box], close);
	});
</script>

<svelte:window onkeydown={keydown} />

<span bind:this={box} class="ml-1 inline-flex">
	<Badge tone="outline" label="+{count} more" title={label} expanded={open} onclick={toggle} />
	{#if open}
		<span
			bind:this={popup}
			data-popup
			role="dialog"
			aria-label={label}
			tabindex="-1"
			class={[popupClasses, 'fixed z-40 flex max-w-80 flex-wrap gap-1 p-2']}
			style:top="{place.top}px"
			style:left="{place.left}px"
			onkeydown={keydown}
			onclick={(event) => {
				if ((event.target as Element).closest('a')) close(false);
			}}
		>
			{@render children()}
		</span>
	{/if}
</span>
