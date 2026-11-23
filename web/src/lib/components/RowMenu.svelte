<!--
@component
The actions of a table row behind one "⋯" button: a menu of links and buttons, given as data.
Keyboard: the arrows move (and wrap), Home and End go to the ends, Esc closes it and returns to
the button, Tab closes it; a click outside, scrolling or resizing close it too. An item with a
confirmation asks in a ConfirmDialog first; a disabled item stays in the list (arrows reach it)
and says why under its label. The menu is fixed under the button (above it when
there is no room below), so a table's scroll box does not cut it.
-->
<script lang="ts">
	import { tick } from 'svelte';
	import { ApiError } from '#lib/api/client.ts';
	import { movedIndex } from '#lib/roving.ts';
	import { toast } from '#lib/toast.svelte.ts';
	import Button from './Button.svelte';
	import type { ConfirmedAction, Confirmation } from './confirm.ts';
	import ConfirmDialog from './ConfirmDialog.svelte';
	import Icon from './Icon.svelte';
	import type { MenuItem } from './menu.ts';
	import { popupClasses } from './surface.ts';

	type Props = {
		/** What the actions are for, e.g. "Actions for backend": the button's and menu's name. */
		label: string;
		items: MenuItem[];
	};

	let { label, items }: Props = $props();

	const menuId = $props.id();

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
		// The viewport without the scrollbar: position: fixed measures from its edge.
		right = Math.max(margin, document.documentElement.clientWidth - box.right);
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
			try {
				await item.action();
			} catch (failure) {
				// Without a confirmation there is no dialog to show it in.
				toast.show(
					failure instanceof ApiError ? failure.message : 'Something went wrong.',
					'error'
				);
			}
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
		'flex w-full items-center gap-2.5 rounded-control px-2.5 py-[7px] text-left whitespace-nowrap no-underline hover:no-underline focus-ring [--focus-offset:-2px]';
</script>

{#snippet content(item: MenuItem)}
	<span class={item.danger ? undefined : 'text-muted'}><Icon name={item.icon} size={16} /></span>
	{item.label}
{/snippet}

{#snippet disabledItem(item: MenuItem & { disabled: string }, index: number)}
	{@const reasonId = `${menuId}-reason-${index}`}
	<div
		role="menuitem"
		tabindex="-1"
		aria-disabled="true"
		aria-describedby={reasonId}
		class={[itemClasses, 'text-muted']}
	>
		<span><Icon name={item.icon} size={16} /></span>
		<span class="flex flex-col">
			{item.label}
			<span id={reasonId} class="text-xs whitespace-normal">{item.disabled}</span>
		</span>
	</div>
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
		class={[popupClasses, 'fixed z-40 min-w-[200px] text-text']}
		style:top="{top}px"
		style:right="{right}px"
		onkeydown={keydown}
	>
		{#each items as item, index (item.label)}
			{#if item.separatorBefore}
				<hr class="my-1 border-border" />
			{/if}
			{@const tone = item.danger
				? 'text-error hover:bg-error-bg focus-visible:bg-error-bg'
				: 'text-text hover:bg-selected focus-visible:bg-selected'}
			{#if item.disabled}
				{@render disabledItem(item, index)}
			{:else if item.href}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- callers pass resolved paths -->
				<a
					role="menuitem"
					tabindex="-1"
					href={item.href}
					class={[itemClasses, 'cursor-pointer', tone]}
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
					class={[itemClasses, 'cursor-pointer', tone]}
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
