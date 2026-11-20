<!--
@component
The actions of a table row behind one "⋯" button: a menu of links and buttons, given as data.
Keyboard: the arrows move (and wrap), Home and End go to the ends, Esc closes it and returns to
the button, Tab closes it; a click outside, scrolling or resizing close it too. An item with a
confirmation asks in a ConfirmDialog first. The menu is fixed under the button (above it when
there is no room below), so a table's scroll box does not cut it.
-->
<script lang="ts">
	import { tick } from 'svelte';
	import { movedIndex } from '#lib/roving.ts';
	import Button from './Button.svelte';
	import type { ConfirmedAction, Confirmation } from './confirm.ts';
	import ConfirmDialog from './ConfirmDialog.svelte';
	import Icon from './Icon.svelte';
	import type { MenuItem } from './menu.ts';

	type Props = {
		/** What the actions are for, e.g. "Actions for backend": the button's and menu's name. */
		label: string;
		items: MenuItem[];
	};

	let { label, items }: Props = $props();

	const gap = 4;
	const margin = 8;

	let open = $state(false);
	let top = $state(0);
	let right = $state(0);
	let toggleBox: HTMLElement;
	let menu: HTMLElement | undefined = $state();

	// The item whose confirmation is asked.
	let asked: { confirm: Confirmation; action: ConfirmedAction } | undefined = $state();
	let asking = $state(false);

	const toggleButton = () => toggleBox.querySelector('button');
	const menuItems = () => [...(menu?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];

	async function show() {
		const box = toggleBox.getBoundingClientRect();

		top = box.bottom + gap;
		right = Math.max(margin, innerWidth - box.right);
		open = true;
		await tick();

		// No room below but room above: open upwards.
		const height = menu?.offsetHeight ?? 0;

		if (top + height > innerHeight - margin && box.top - height - gap > margin) {
			top = box.top - height - gap;
		}

		menuItems()[0]?.focus();
	}

	function close(focusButton: boolean) {
		open = false;

		if (focusButton) toggleButton()?.focus();
	}

	function keydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			event.preventDefault();
			close(true);

			return;
		}

		if (event.key === 'Tab') {
			close(false);

			return;
		}

		const list = menuItems();
		const index = movedIndex(
			event.key,
			list.indexOf(document.activeElement as HTMLElement),
			list.length,
			'vertical'
		);

		if (index !== undefined) {
			event.preventDefault();
			list[index].focus();
		}
	}

	async function choose(item: MenuItem) {
		// Back on the button first: a dialog returns the focus where it was when it opened.
		close(true);

		if (!item.action) return;

		if (item.confirm) {
			asked = { confirm: item.confirm, action: item.action };
			asking = true;
		} else {
			await item.action();
		}
	}

	// A click outside, scrolling or resizing close an open menu.
	$effect(() => {
		if (!open) return;

		const closeOutside = (event: Event) => {
			const target = event.target as Node;

			if (!menu?.contains(target) && !toggleBox.contains(target)) close(false);
		};
		const closeNow = () => close(false);

		document.addEventListener('pointerdown', closeOutside);
		addEventListener('scroll', closeNow, true);
		addEventListener('resize', closeNow);

		return () => {
			document.removeEventListener('pointerdown', closeOutside);
			removeEventListener('scroll', closeNow, true);
			removeEventListener('resize', closeNow);
		};
	});

	const itemClasses =
		'flex w-full cursor-pointer items-center gap-2.5 rounded-control px-2.5 py-[7px] text-left whitespace-nowrap no-underline outline-none hover:no-underline';
</script>

{#snippet content(item: MenuItem)}
	<span class={item.danger ? undefined : 'text-muted'}><Icon name={item.icon} size={16} /></span>
	{item.label}
{/snippet}

<span bind:this={toggleBox} class="inline-flex">
	<Button
		variant="ghost"
		icon="more"
		title={label}
		popup="menu"
		expanded={open}
		onclick={() => (open ? close(false) : show())}
	/>
</span>

{#if open}
	<div
		bind:this={menu}
		role="menu"
		aria-label={label}
		tabindex="-1"
		data-popup
		class="fixed z-40 min-w-[200px] rounded-popup border border-border bg-surface p-1 text-text shadow-pop forced-colors:border"
		style:top="{top}px"
		style:right="{right}px"
		onkeydown={keydown}
	>
		{#each items as item (item.label)}
			{#if item.separatorBefore}
				<hr class="my-1 border-border" />
			{/if}
			{@const tone = item.danger
				? 'text-error hover:bg-error-bg focus:bg-error-bg'
				: 'text-text hover:bg-selected focus:bg-selected'}
			{#if item.href}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- callers pass resolved paths -->
				<a
					role="menuitem"
					tabindex="-1"
					href={item.href}
					class={[itemClasses, tone]}
					onclick={() => close(false)}
				>
					{@render content(item)}
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{:else}
				<button
					type="button"
					role="menuitem"
					tabindex="-1"
					class={[itemClasses, tone]}
					onclick={() => choose(item)}
				>
					{@render content(item)}
				</button>
			{/if}
		{/each}
	</div>
{/if}

{#if asked}
	<ConfirmDialog bind:open={asking} {...asked.confirm} onconfirm={asked.action} />
{/if}
