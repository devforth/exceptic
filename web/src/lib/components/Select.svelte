<!--
@component
One choice from a list: a button that opens a listbox, the chosen option checked; `all` adds a
first option for no choice (its value is empty). The button is as wide as its longest option;
as a `field` of a form its label shows above it and it is as wide as the other fields.
Keyboard: Down, Up, Enter and Space open it on the chosen option; in it the arrows move, Home
and End go to the ends, a letter goes to the next option starting with it, Enter or Space
choose, Esc closes it back on the button, Tab closes it. The list is fixed under the button
(popup.ts), so a table's scroll box does not cut it; a click outside, scrolling or resizing
close it.
-->
<script lang="ts">
	import { tick } from 'svelte';
	import { controlClasses, controlHoverClasses } from './field.ts';
	import Icon from './Icon.svelte';
	import {
		chevronClasses,
		closeWhenLeft,
		listMinWidth,
		openControlClasses,
		popupPlace,
		type PopupPlace
	} from './popup.ts';
	import type { SelectOption } from './select.ts';
	import { popupClasses } from './surface.ts';

	type Props = {
		/** Its accessible name, e.g. "Rows per page". */
		label: string;
		value: string;
		options: readonly SelectOption[];
		onchange: (value: string) => void;
		/** The option of no choice, e.g. "All statuses". */
		all?: string;
		/** A field of a form: its label shown above it, as wide as the form's fields. */
		field?: boolean;
	};

	let { label, value, options, onchange, all, field = false }: Props = $props();

	const id = $props.id();

	const choices = $derived(all === undefined ? options : [{ value: '', label: all }, ...options]);
	const chosen = $derived(choices.find((choice) => choice.value === value));

	let open = $state(false);
	let place: PopupPlace = $state({ top: 0, left: 0, minWidth: 0 });
	let button: HTMLButtonElement;
	let list: HTMLElement | undefined = $state();

	const items = () => [...(list?.querySelectorAll<HTMLElement>('[role="option"]') ?? [])];

	async function show() {
		open = true;
		await tick();

		place = popupPlace(button.getBoundingClientRect(), {
			width: list?.offsetWidth ?? 0,
			height: list?.offsetHeight ?? 0
		});

		const elements = items();

		(elements[choices.findIndex((choice) => choice.value === value)] ?? elements[0])?.focus();
	}

	function close(focusButton: boolean) {
		open = false;

		if (focusButton) button.focus();
	}

	function choose(choice: SelectOption) {
		close(true);

		if (choice.value !== value) onchange(choice.value);
	}

	function buttonKeydown(event: KeyboardEvent) {
		if (!open && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
			event.preventDefault();
			void show();
		}
	}

	function listKeydown(event: KeyboardEvent) {
		const elements = items();
		const at = elements.indexOf(document.activeElement as HTMLElement);
		const go = (index: number) => {
			event.preventDefault();
			elements[Math.max(0, Math.min(elements.length - 1, index))]?.focus();
		};

		switch (event.key) {
			case 'ArrowDown':
				return go(at + 1);
			case 'ArrowUp':
				return go(at - 1);
			case 'Home':
				return go(0);
			case 'End':
				return go(elements.length - 1);
			case 'Enter':
			case ' ':
				event.preventDefault();
				if (at >= 0) choose(choices[at]);

				return;
			case 'Escape':
				// Handled: Esc must not also leave the page (Breadcrumbs).
				event.preventDefault();

				return close(true);
			case 'Tab':
				return close(false);
		}

		if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
			const letter = event.key.toLowerCase();
			const starts = (index: number) => choices[index].label.toLowerCase().startsWith(letter);
			const indexes = choices.map((_, index) => index);
			const next = indexes.find((index) => index > at && starts(index)) ?? indexes.find(starts);

			if (next !== undefined) go(next);
		}
	}

	$effect(() => {
		if (!open) return;

		return closeWhenLeft(() => [list, button], close);
	});
</script>

{#snippet trigger()}
	<button
		bind:this={button}
		type="button"
		aria-haspopup="listbox"
		aria-expanded={open}
		aria-controls={open ? `${id}-list` : undefined}
		aria-labelledby="{id}-label {id}-value"
		class={[
			controlClasses,
			'inline-flex min-w-0 cursor-pointer items-center justify-between gap-2.5 text-left whitespace-nowrap transition-[background-color,border-color,box-shadow]',
			field && 'w-full',
			open ? openControlClasses : `${controlHoverClasses} hover:bg-hover`
		]}
		onclick={() => (open ? close(false) : show())}
		onkeydown={buttonKeydown}
	>
		<!-- Every label in one cell: the button takes the width of the longest. -->
		<span class="grid min-w-0">
			{#each choices as choice (choice.value)}
				<span class="invisible col-start-1 row-start-1" aria-hidden="true">{choice.label}</span>
			{/each}
			<span id="{id}-value" class="col-start-1 row-start-1 truncate">{chosen?.label ?? ''}</span>
		</span>
		<span class={chevronClasses(open)}>
			<Icon name="expand" size={16} />
		</span>
	</button>
{/snippet}

{#if field}
	<div class="flex flex-col gap-1">
		<span id="{id}-label" class="text-label text-muted">{label}</span>
		{@render trigger()}
	</div>
{:else}
	<span id="{id}-label" class="sr-only">{label}</span>
	{@render trigger()}
{/if}

{#if open}
	<div
		bind:this={list}
		id="{id}-list"
		role="listbox"
		aria-labelledby="{id}-label"
		tabindex="-1"
		data-popup
		class={[popupClasses, 'fixed z-40 text-sm text-text']}
		style:top="{place.top}px"
		style:left="{place.left}px"
		style:min-width="{Math.max(place.minWidth, listMinWidth)}px"
		onkeydown={listKeydown}
	>
		{#each choices as choice (choice.value)}
			{@const selected = choice.value === value}
			<!-- The listbox handles the keys. -->
			<!-- svelte-ignore a11y_click_events_have_key_events -->
			<div
				role="option"
				tabindex="-1"
				aria-selected={selected}
				class={[
					'flex cursor-pointer items-center justify-between gap-4 rounded-control px-2.5 py-[7px] whitespace-nowrap outline-none hover:bg-accent-soft focus:bg-accent-soft forced-colors:focus:outline-2',
					selected && 'font-semibold text-accent'
				]}
				onclick={() => choose(choice)}
			>
				{choice.label}
				<span class={selected ? 'flex' : 'invisible flex'}><Icon name="check" size={16} /></span>
			</div>
		{/each}
	</div>
{/if}
